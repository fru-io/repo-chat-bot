package pkg

import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	siteapi "github.com/drud/ddev-live-sdk/golang/pkg/site/apis/site/v1beta1"
	siteclientset "github.com/drud/ddev-live-sdk/golang/pkg/site/clientset"
	sitefactory "github.com/drud/ddev-live-sdk/golang/pkg/site/informers/externalversions"
	sitelister "github.com/drud/ddev-live-sdk/golang/pkg/site/listers/site/v1beta1"

	drupalclientset "github.com/drud/ddev-live-sdk/golang/pkg/drupal/clientset"
	drupalfactory "github.com/drud/ddev-live-sdk/golang/pkg/drupal/informers/externalversions"
)

const (
	botAnnotation  = "ddev.live/endor-bot"
	prAnnotation   = "ddev.live/endor-bot-pr"
	repoAnnotation = "ddev.live/endor-bot-repo"
	chanSize       = 1024
)

type Bot interface {
	Response(args ResponseRequest) string
	ReceiveUpdate() (UpdateEvent, error)
}

type bot struct {
	// this is used to determine which sites are build by which operator
	annotation   string
	scWatcher    scWatcher
	cmsWatcher   cmsWatcher
	updateEvents chan UpdateEvent

	siteClientSet *siteclientset.Clientset

	sisLister   sitelister.SiteImageSourceLister
	sisInformer cache.SharedIndexInformer

	scLister   sitelister.SiteCloneLister
	scInformer cache.SharedIndexInformer
}

var (
	defaultResyncPeriod = time.Minute * 30
)

func InitBot(kubeconfig *restclient.Config, annotation string, stopCh <-chan struct{}) (Bot, error) {
	scs, err := siteclientset.NewForConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	drupal, err := drupalclientset.NewForConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	sf := sitefactory.NewSharedInformerFactory(scs, defaultResyncPeriod)
	sisInformer := sf.Site().V1beta1().SiteImageSources().Informer()
	sisLister := sf.Site().V1beta1().SiteImageSources().Lister()
	scInformer := sf.Site().V1beta1().SiteClones().Informer()
	scLister := sf.Site().V1beta1().SiteClones().Lister()

	df := drupalfactory.NewSharedInformerFactory(drupal, defaultResyncPeriod)
	dLister := df.Cms().V1beta1().DrupalSites().Lister()
	dInformer := df.Cms().V1beta1().DrupalSites().Informer()
	updateEvents := make(chan UpdateEvent, chanSize)

	scWatcher := scWatcher{
		annotation:   annotation,
		scLister:     scLister,
		updateEvents: updateEvents,
	}
	scInformer.AddEventHandler(scWatcher)
	cmsWatcher := cmsWatcher{
		annotation:   annotation,
		scLister:     scLister,
		dLister:      dLister,
		updateEvents: updateEvents,
	}
	dInformer.AddEventHandler(cmsWatcher)

	sf.Start(stopCh)
	sf.WaitForCacheSync(stopCh)
	df.Start(stopCh)
	df.WaitForCacheSync(stopCh)

	return &bot{
		annotation:    annotation,
		scWatcher:     scWatcher,
		cmsWatcher:    cmsWatcher,
		updateEvents:  updateEvents,
		siteClientSet: scs,
		sisInformer:   sisInformer,
		sisLister:     sisLister,
		scInformer:    scInformer,
		scLister:      scLister,
	}, nil
}

func (b *bot) Response(args ResponseRequest) string {
	lines := strings.Split(strings.Replace(args.Body, "\r\n", "\n", -1), "\n")
	resp := make(map[string]string)
	for _, line := range lines {
		if !strings.HasPrefix(line, commandPrefix) {
			continue
		}
		switch line {
		case ping:
			resp[line] = pong
		case previewSite:
			resp[line] = b.previewSite(args)
		default:
			resp[line] = fmt.Sprintf("Unknown command: `%v`", line)
		}
	}
	var r []string
	for _, msg := range resp {
		r = append(r, msg)
	}
	return strings.Join(r, "\n\n")
}

type ResponseRequest struct {
	Body         string
	RepoURL      string
	Namespace    string
	OriginBranch string
	CloneBranch  string
	PR           int
	Annotations  map[string]string
}

type UpdateEvent struct {
	Message     string
	PR          int
	RepoURL     string
	Type        string
	Annotations map[string]string
}

func sisMatches(sis *siteapi.SiteImageSource, repoURL, originBranch string) bool {
	if sis.Spec.GitSource.URL == repoURL && sis.Spec.GitSource.Revision == originBranch {
		return true
	}
	parsed, err := url.Parse(repoURL)
	if err != nil {
		klog.Errorf("failed to parse URL %q: %v", repoURL, err)
		return false
	}
	if parsed.Hostname() != "github.com" {
		return false
	}
	split := strings.Split(parsed.Path, "/")
	if len(split) != 3 {
		klog.Errorf("failed to parse path %q: %v-%v", parsed.Path, len(split), split)
		return false
	}
	repo := split[2]
	org := split[1]
	return sis.Spec.GitHubSourceDeprecated.RepoName == repo &&
		sis.Spec.GitHubSourceDeprecated.OrgName == org &&
		sis.Spec.GitHubSourceDeprecated.Branch == originBranch
}

func filterSis(list []*siteapi.SiteImageSource, repoURL, originBranch string) []*siteapi.SiteImageSource {
	var filtered []*siteapi.SiteImageSource
	for _, sis := range list {
		if sisMatches(sis, repoURL, originBranch) {
			filtered = append(filtered, sis)
		}
	}
	return filtered
}

func siteClone(sis *siteapi.SiteImageSource, cloneBranch string, pr int, annotations map[string]string) *siteapi.SiteClone {
	return &siteapi.SiteClone{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   sis.Namespace,
			Name:        fmt.Sprintf("%v-pr%v", sis.Name, pr),
			Annotations: annotations,
		},
		Spec: siteapi.SiteCloneSpec{
			Origin: siteapi.OriginSpec{
				Name: sis.Name,
			},
			Clone: siteapi.CloneSpec{
				Name:     fmt.Sprintf("%v-pr%v", sis.Name, pr),
				Revision: cloneBranch,
			},
		},
	}
}

func (b *bot) previewSite(args ResponseRequest) string {
	list, err := b.sisLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("failed listing SiteImageSource: %v", err)
		return previewGenericError
	}
	filtered := filterSis(list, args.RepoURL, args.OriginBranch)
	var msgs []string
	args.Annotations[botAnnotation] = b.annotation
	args.Annotations[prAnnotation] = fmt.Sprintf("%d", args.PR)
	args.Annotations[repoAnnotation] = args.RepoURL
	for _, sis := range filtered {
		siteclone := siteClone(sis, args.CloneBranch, args.PR, args.Annotations)
		if sc, err := b.scLister.SiteClones(siteclone.Namespace).Get(sis.Name); err == nil {
			msgs = append(msgs, previewCreating(sc))
			continue
		}
		if sc, err := b.siteClientSet.SiteV1beta1().SiteClones(sis.Namespace).Create(siteclone); err != nil {
			if !kerrors.IsAlreadyExists(err) {
				klog.Errorf("failed to create site clone %v: %v", siteclone, err)
				msgs = append(msgs, fmt.Sprintf(previewSiteError, siteclone.Spec.Origin.Name))
				continue
			}
			msgs = append(msgs, previewCreating(sc))
		} else {
			msgs = append(msgs, previewCreating(sc))
		}
	}
	if len(msgs) == 0 {
		msgs = append(msgs, fmt.Sprintf(previewSiteNoOrigin, args.OriginBranch))
	}
	return strings.Join(msgs, "\n\n")
}

func (b *bot) ReceiveUpdate() (UpdateEvent, error) {
	for msg := range b.scWatcher.updateEvents {
		return msg, nil
	}
	return UpdateEvent{}, io.EOF
}
