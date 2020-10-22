package pkg

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	siteclientset "github.com/drud/ddev-live-sdk/golang/pkg/site/clientset"
	sitefactory "github.com/drud/ddev-live-sdk/golang/pkg/site/informers/externalversions"
	sitelister "github.com/drud/ddev-live-sdk/golang/pkg/site/listers/site/v1beta1"

	drupalclientset "github.com/drud/ddev-live-sdk/golang/pkg/drupal/clientset"
	drupalfactory "github.com/drud/ddev-live-sdk/golang/pkg/drupal/informers/externalversions"
	drupallister "github.com/drud/ddev-live-sdk/golang/pkg/drupal/listers/cms/v1beta1"

	typo3clientset "github.com/drud/ddev-live-sdk/golang/pkg/typo3/clientset"
	typo3factory "github.com/drud/ddev-live-sdk/golang/pkg/typo3/informers/externalversions"
	typo3lister "github.com/drud/ddev-live-sdk/golang/pkg/typo3/listers/cms/v1beta1"

	wordpressclientset "github.com/drud/ddev-live-sdk/golang/pkg/wordpress/clientset"
	wordpressfactory "github.com/drud/ddev-live-sdk/golang/pkg/wordpress/informers/externalversions"
	wordpresslister "github.com/drud/ddev-live-sdk/golang/pkg/wordpress/listers/cms/v1beta1"

	drupalapi "github.com/drud/ddev-live-sdk/golang/pkg/drupal/apis/cms/v1beta1"
	siteapi "github.com/drud/ddev-live-sdk/golang/pkg/site/apis/site/v1beta1"
	typo3api "github.com/drud/ddev-live-sdk/golang/pkg/typo3/apis/cms/v1beta1"
	wordpressapi "github.com/drud/ddev-live-sdk/golang/pkg/wordpress/apis/cms/v1beta1"
)

const (
	botAnnotation  = "ddev.live/repo-chat-bot"
	prAnnotation   = "ddev.live/repo-chat-bot-pr"
	repoAnnotation = "ddev.live/repo-chat-bot-repo"
	chanSize       = 1024
)

type Bot interface {
	Response(args ResponseRequest) string
	ReceiveUpdate() (UpdateEvent, error)
}

type bot struct {
	scWatcher    scWatcher
	cmsWatcher   cmsWatcher
	updateEvents chan UpdateEvent

	kubeClients
}

type kubeClients struct {
	// this is used to determine which sites are build by which operator
	annotation string

	// clientset is used for creating resources
	siteClientSet *siteclientset.Clientset

	// listers and informers form cache for resources
	// site API listers
	sisLister   sitelister.SiteImageSourceLister
	sisInformer cache.SharedIndexInformer
	scLister    sitelister.SiteCloneLister
	scInformer  cache.SharedIndexInformer

	// cms API listers
	dLister   drupallister.DrupalSiteLister
	dInformer cache.SharedIndexInformer
	tLister   typo3lister.Typo3SiteLister
	tInformer cache.SharedIndexInformer
	wLister   wordpresslister.WordpressLister
	wInformer cache.SharedIndexInformer
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
	typo3, err := typo3clientset.NewForConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	wordpress, err := wordpressclientset.NewForConfig(kubeconfig)
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

	tf := typo3factory.NewSharedInformerFactory(typo3, defaultResyncPeriod)
	tLister := tf.Cms().V1beta1().Typo3Sites().Lister()
	tInformer := tf.Cms().V1beta1().Typo3Sites().Informer()

	wf := wordpressfactory.NewSharedInformerFactory(wordpress, defaultResyncPeriod)
	wLister := wf.Cms().V1beta1().Wordpresses().Lister()
	wInformer := wf.Cms().V1beta1().Wordpresses().Informer()

	updateEvents := make(chan UpdateEvent, chanSize)

	kubeClients := kubeClients{
		annotation:    annotation,
		siteClientSet: scs,
		sisInformer:   sisInformer,
		sisLister:     sisLister,
		scInformer:    scInformer,
		scLister:      scLister,
		dLister:       dLister,
		dInformer:     dInformer,
		tLister:       tLister,
		tInformer:     tInformer,
		wLister:       wLister,
		wInformer:     wInformer,
	}
	scWatcher := scWatcher{
		updateEvents: updateEvents,
		kubeClients:  kubeClients,
	}
	scInformer.AddEventHandler(scWatcher)
	cmsWatcher := cmsWatcher{
		updateEvents: updateEvents,
		kubeClients:  kubeClients,
	}
	dInformer.AddEventHandler(cmsWatcher)

	sf.Start(stopCh)
	sf.WaitForCacheSync(stopCh)
	df.Start(stopCh)
	df.WaitForCacheSync(stopCh)

	return &bot{
		scWatcher:    scWatcher,
		cmsWatcher:   cmsWatcher,
		updateEvents: updateEvents,
		kubeClients:  kubeClients,
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
		if sc, err := b.scLister.SiteClones(siteclone.Namespace).Get(siteclone.Name); err == nil {
			msg, _, _ := b.previewSiteUpdateFromSiteClone(sc)
			msgs = append(msgs, msg)
			continue
		}
		if sc, err := b.siteClientSet.SiteV1beta1().SiteClones(sis.Namespace).Create(siteclone); err != nil {
			if !kerrors.IsAlreadyExists(err) {
				klog.Errorf("failed to create site clone %v: %v", siteclone, err)
				msgs = append(msgs, fmt.Sprintf(previewSiteError, siteclone.Spec.Origin.Name))
				continue
			}
			msg, _, _ := b.previewSiteUpdateFromSiteClone(sc)
			msgs = append(msgs, msg)
		} else {
			msg, _, _ := b.previewSiteUpdateFromSiteClone(sc)
			msgs = append(msgs, msg)
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

func (k kubeClients) getSiteStatus(namespace, name string) (siteStatus, error) {
	ds, err := k.dLister.DrupalSites(namespace).Get(name)
	if err == nil {
		return siteStatus{conditions: ds.Status.Conditions, webStatus: ds.Status.WebStatus}, nil
	}
	ts, err := k.tLister.Typo3Sites(namespace).Get(name)
	if err == nil {
		return getCommonStatus(ts), nil
	}
	ws, err := k.wLister.Wordpresses(namespace).Get(name)
	if err == nil {
		return siteStatus{conditions: ws.Status.Conditions, webStatus: ws.Status.WebStatus}, nil
	}
	return siteStatus{}, fmt.Errorf("Site %v/%v not found", namespace, name)
}

func (k kubeClients) previewSiteUpdateFromSiteClone(sc *siteapi.SiteClone) (string, int, error) {
	if sc.Annotations[botAnnotation] != k.annotation {
		return "", 0, nil
	}
	pr, err := strconv.Atoi(sc.Annotations[prAnnotation])
	if err != nil {
		return previewGenericError, 0, fmt.Errorf("failed to parse %q annotation: %v", prAnnotation, err)
	}

	ss, err := k.getSiteStatus(sc.Namespace, sc.Spec.Clone.Name)
	if err != nil {
		klog.V(2).Info("Site %v/%v not found yet", sc.Namespace, sc.Spec.Clone.Name)
	}
	msg := previewCreating(sc, ss)
	return msg, pr, nil
}

func (k kubeClients) previewSiteUpdateFromCMS(obj metav1.Object) (string, int, error) {
	var sc *siteapi.SiteClone
	for _, o := range obj.GetOwnerReferences() {
		if o.Kind == "SiteClone" {
			c, err := k.scLister.SiteClones(obj.GetNamespace()).Get(o.Name)
			if err == nil {
				sc = c
				break
			}
		}
	}
	if sc == nil {
		err := fmt.Errorf("failed to find SiteClone from owner references in %T, %v/%v", obj, obj.GetNamespace(), obj.GetName())
		return previewGenericError, 0, err
	}
	return k.previewSiteUpdateFromSiteClone(sc)
}

func (k kubeClients) previewSiteUpdate(obj metav1.Object) (string, int, error) {
	switch o := obj.(type) {
	case *siteapi.SiteClone:
		return k.previewSiteUpdateFromSiteClone(o)
	case *drupalapi.DrupalSite, *typo3api.Typo3Site, *wordpressapi.Wordpress:
		return k.previewSiteUpdateFromCMS(o)
	default:
		return "", 0, fmt.Errorf("unsupported type %T for preview site update", o)
	}
	return "", 0, nil
}
