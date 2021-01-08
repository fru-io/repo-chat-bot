/*
Copyright DDEV Technologies LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pkg

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	common "github.com/drud/cms-common/api/v1beta1"
	git "github.com/whilp/git-urls"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubefactory "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelister "k8s.io/client-go/listers/core/v1"
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

	pbAdmin "github.com/drud/admin-api/gen/live/administration/v1alpha1"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	grpcmeta "google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"
)

const (
	prAnnotation   = "ddev.live/repo-chat-bot-pr"
	repoAnnotation = "ddev.live/repo-chat-bot-repo"
	botAnnotation  = "ddev.live/repo-chat-bot"
	chanSize       = 1024
)

var (
	defaultResyncPeriod = time.Minute * 30
	logLimitBytes       = int64(4 * 1024)
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
	// clone site name suffix
	siteSuffix string
	// used to wait for all informer caches to get synced
	wait chan bool

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

	// core API client and lister
	coreClientSet *kubernetes.Clientset
	podLister     corelister.PodLister

	// gRPC auth admin service
	authClient pbAdmin.AdministrationClient
}

func authAdminAPI(k *kubernetes.Clientset, authSVCLabels string) (pbAdmin.AdministrationClient, error) {
	svcs, err := k.CoreV1().Services("").List(metav1.ListOptions{LabelSelector: authSVCLabels})
	if err != nil {
		return nil, err
	}
	if len(svcs.Items) != 1 {
		return nil, fmt.Errorf("failed to lookup admin gRPC service for labels %v, expected exactly 1, found %v", authSVCLabels, len(svcs.Items))
	}
	svc := svcs.Items[0]
	port := int32(8443)
	for _, p := range svc.Spec.Ports {
		if p.Name == "grpc" {
			port = p.Port
			break
		}
	}
	grpcURL := fmt.Sprintf("%v.%v.svc:%v", svc.Name, svc.Namespace, port)
	conn, err := grpc.Dial(grpcURL, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return pbAdmin.NewAdministrationClient(conn), nil
}

func InitBot(kubeconfig *restclient.Config, annotation, siteSuffix string, authSVCLabels string, stopCh <-chan struct{}) (Bot, error) {
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
	kcs, err := kubernetes.NewForConfig(kubeconfig)
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

	kf := kubefactory.NewSharedInformerFactory(kcs, defaultResyncPeriod)
	podLister := kf.Core().V1().Pods().Lister()
	adminClient, err := authAdminAPI(kcs, authSVCLabels)
	if err != nil {
		return nil, err
	}

	updateEvents := make(chan UpdateEvent, chanSize)

	kubeClients := kubeClients{
		annotation:    annotation,
		siteSuffix:    siteSuffix,
		wait:          make(chan bool, 1),
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
		podLister:     podLister,
		coreClientSet: kcs,
		authClient:    adminClient,
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
	tInformer.AddEventHandler(cmsWatcher)
	wInformer.AddEventHandler(cmsWatcher)

	df.Start(stopCh)
	tf.Start(stopCh)
	wf.Start(stopCh)
	sf.Start(stopCh)
	kf.Start(stopCh)
	df.WaitForCacheSync(stopCh)
	tf.WaitForCacheSync(stopCh)
	wf.WaitForCacheSync(stopCh)
	sf.WaitForCacheSync(stopCh)
	kf.WaitForCacheSync(stopCh)
	close(kubeClients.wait)

	return &bot{
		scWatcher:    scWatcher,
		cmsWatcher:   cmsWatcher,
		updateEvents: updateEvents,
		kubeClients:  kubeClients,
	}, nil
}

func (b *bot) Response(args ResponseRequest) string {
	if IsBotMessage(args.Body) {
		return ""
	}
	lines := strings.Split(strings.Replace(args.Body, "\r\n", "\n", -1), "\n")
	resp := make(map[string]string)
	for _, line := range lines {
		if !strings.HasPrefix(line, commandPrefix) {
			continue
		}
		var r string
		switch line {
		case Ping:
			r = pong
		case Help:
			r = b.helpResponse(args, true)
		case HelpOnPROpen:
			r = b.helpResponse(args, false)
		case PreviewSite:
			r = b.previewSite(args)
		case DeletePreviewSite:
			r = b.deletePreviewSite(args, true)
		case ClosePreviewSite:
			r = b.deletePreviewSite(args, false)
		default:
			r = fmt.Sprintf("Unknown command: `%v`", line)
		}
		if len(r) != 0 {
			resp[line] = r
		}
	}
	var r []string
	for _, msg := range resp {
		r = append(r, msg)
	}
	if len(r) == 0 {
		return ""
	} else {
		return fmt.Sprintf("%v\n%v", MessageMarker, strings.Join(r, "\n\n"))
	}
}

type ResponseRequest struct {
	Email        string
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

func RepoURLNormalize(url string) string {
	url = strings.TrimSuffix(url, ".git")
	u, err := git.Parse(url)
	if err != nil {
		return url
	}
	return fmt.Sprintf("%v/%v", u.Hostname(), strings.TrimLeft(u.Path, "/"))
}

func sisMatches(sis *siteapi.SiteImageSource, repoURL, originBranch string) bool {
	if RepoURLNormalize(sis.Spec.GitSource.URL) == RepoURLNormalize(repoURL) && sis.Spec.GitSource.Revision == originBranch {
		return true
	}
	parsed, err := git.Parse(repoURL)
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

func filterSc(list []*siteapi.SiteClone, repoURL string, pr int) []*siteapi.SiteClone {
	var filtered []*siteapi.SiteClone
	prStr := fmt.Sprintf("%d", pr)
	for _, sc := range list {
		if sc.Annotations != nil &&
			sc.Annotations[repoAnnotation] == repoURL &&
			sc.Annotations[prAnnotation] == prStr {
			filtered = append(filtered, sc)
		}
	}
	return filtered
}

func siteClone(site metav1.Object, cloneBranch string, pr int, annotations map[string]string, suffix string) *siteapi.SiteClone {
	return &siteapi.SiteClone{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   site.GetNamespace(),
			Name:        fmt.Sprintf("%v-%v%v", site.GetName(), suffix, pr),
			Annotations: annotations,
		},
		Spec: siteapi.SiteCloneSpec{
			Origin: siteapi.OriginSpec{
				Name: site.GetName(),
			},
			Clone: siteapi.CloneSpec{
				Name:     fmt.Sprintf("%v-%v%v", site.GetName(), suffix, pr),
				Revision: cloneBranch,
			},
		},
	}
}

func (b *bot) helpResponse(args ResponseRequest, verbose bool) string {
	if verbose {
		// display bot help message when user asks for it even when there are no origin to clone from
		HelpPreviewSiteCounter.WithLabelValues(b.annotation, b.siteSuffix).Inc()
		return helpResponse
	}
	HelpPreviewSiteCounter.WithLabelValues(b.annotation, metricLabelCommand).Inc()

	list, err := b.sisLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("failed listing SiteImageSource: %v", err)
		// don't display any bot help message on repos that don't have origin to clone from
		return ""
	}
	filtered := filterSis(list, args.RepoURL, args.OriginBranch)
	if len(filtered) == 0 {
		// don't display any bot help message on repos that don't have origin to clone from
		return ""
	}
	return helpResponse
}

func (b *bot) deletePreviewSite(args ResponseRequest, verboseErrors bool) string {
	ml := b.siteSuffix
	if verboseErrors {
		ml = metricLabelCommand
	}
	list, err := b.scLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("failed listing SiteClones: %v", err)
		if verboseErrors {
			return previewGenericError
		} else {
			return ""
		}
		DeletePreviewSiteCounter.WithLabelValues(b.annotation, ml, metricLabelPreviewFailed).Inc()
	}
	filtered := filterSc(list, args.RepoURL, args.PR)
	var msgs []string
	for _, sc := range filtered {
		if verboseErrors {
			if allow, err := b.hasPreviewSiteCapability(args.Email, sc.Namespace); err != nil {
				DeletePreviewSiteCounter.WithLabelValues(b.annotation, ml, metricLabelPreviewAuthAPIError).Inc()
				msgs = append(msgs, previewGenericError)
				continue
			} else if !allow {
				DeletePreviewSiteCounter.WithLabelValues(b.annotation, ml, metricLabelPreviewDenied).Inc()
				msgs = append(msgs, fmt.Sprintf(previewDenied, args.Email, sc.Namespace))
				continue
			}
		}
		if err := b.siteClientSet.SiteV1beta1().SiteClones(sc.Namespace).Delete(sc.Name, nil); err != nil && !kerrors.IsNotFound(err) {
			// error asking API to delete SiteClone other than IsNotFound
			klog.Errorf("failed to delete SiteClone %v/%v: %v", sc.Namespace, sc.Name, err)
			if verboseErrors {
				msgs = append(msgs, fmt.Sprintf(deleteSiteError, sc.Spec.Clone.Name, sc.Namespace))
			}
			DeletePreviewSiteCounter.WithLabelValues(b.annotation, ml, metricLabelPreviewFailed).Inc()
		} else if err == nil {
			// no error, we have successfully deleted SiteClone
			DeletePreviewSiteCounter.WithLabelValues(b.annotation, ml, metricLabelPreviewSuccess).Inc()
			msgs = append(msgs, fmt.Sprintf(deleteSite, sc.Spec.Clone.Name, sc.Namespace))
		}
	}
	if len(msgs) == 0 && verboseErrors {
		return fmt.Sprintf("%v\n___\n%v", deleteSiteNone, helpResponse)
	}
	return strings.Join(msgs, "\n\n")
}

func (b *bot) hasPreviewSiteCapability(email, org string) (bool, error) {
	ctx := context.Background()
	metaCtx := grpcmeta.AppendToOutgoingContext(ctx, "x-ddev-workspace", org)
	caps, err := b.authClient.ListCapabilities(metaCtx, &pbAdmin.ListCapabilitiesRequest{Email: email})
	if err != nil {
		if grpcstatus.Code(err) == grpccodes.NotFound {
			return false, nil
		}
		klog.Errorf("failed querying capabilities for user %v: %v", email, err)
		return false, err
	}
	for _, c := range caps.Capabilities {
		if c == pbAdmin.Capability_SiteEditor {
			return true, nil
		}
	}
	return false, nil
}

func (b *bot) previewSite(args ResponseRequest) string {
	if args.Email == "" {
		CreatePreviewSiteCounter.WithLabelValues(b.annotation, metricLabelPreviewDeniedMissingEmail).Inc()
		return previewDeniedMissingEmail
	}
	list, err := b.sisLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("failed listing SiteImageSource: %v", err)
		CreatePreviewSiteCounter.WithLabelValues(b.annotation, metricLabelPreviewFailed).Inc()
		return previewGenericError
	}
	filtered := filterSis(list, args.RepoURL, args.OriginBranch)
	var msgs []string
	args.Annotations[botAnnotation] = b.annotation
	args.Annotations[prAnnotation] = fmt.Sprintf("%d", args.PR)
	args.Annotations[repoAnnotation] = args.RepoURL
	for _, sis := range filtered {
		site, err := b.getSiteForSIS(sis)
		if err != nil {
			klog.Errorf("failed to find site for SiteImageSource %v/%v: %v", sis.Namespace, sis.Name, err)
			continue
		}
		if allow, err := b.hasPreviewSiteCapability(args.Email, sis.Namespace); err != nil {
			msgs = append(msgs, previewGenericError)
			CreatePreviewSiteCounter.WithLabelValues(b.annotation, metricLabelPreviewAuthAPIError).Inc()
			continue
		} else if !allow {
			msgs = append(msgs, fmt.Sprintf(previewDenied, args.Email, sis.Namespace))
			CreatePreviewSiteCounter.WithLabelValues(b.annotation, metricLabelPreviewDenied).Inc()
			continue
		}
		siteclone := siteClone(site, args.CloneBranch, args.PR, args.Annotations, b.siteSuffix)
		if sc, err := b.scLister.SiteClones(siteclone.Namespace).Get(siteclone.Name); err == nil {
			CreatePreviewSiteCounter.WithLabelValues(b.annotation, metricLabelPreviewSuccess).Inc()
			ue, _ := b.previewSiteUpdate(sc)
			msgs = append(msgs, ue.Message)
			continue
		}
		if sc, err := b.siteClientSet.SiteV1beta1().SiteClones(sis.Namespace).Create(siteclone); err != nil {
			if !kerrors.IsAlreadyExists(err) {
				klog.Errorf("failed to create site clone %v: %v", siteclone, err)
				CreatePreviewSiteCounter.WithLabelValues(b.annotation, metricLabelPreviewFailed).Inc()
				msgs = append(msgs, fmt.Sprintf(previewSiteError, siteclone.Spec.Origin.Name))
				continue
			}
			CreatePreviewSiteCounter.WithLabelValues(b.annotation, metricLabelPreviewSuccess).Inc()
			ue, _ := b.previewSiteUpdate(sc)
			msgs = append(msgs, ue.Message)
		} else {
			CreatePreviewSiteCounter.WithLabelValues(b.annotation, metricLabelPreviewSuccess).Inc()
			ue, _ := b.previewSiteUpdate(sc)
			msgs = append(msgs, ue.Message)
		}
	}
	if len(msgs) == 0 {
		msgs = append(msgs, fmt.Sprintf(previewSiteNoOrigin, args.OriginBranch))
	}
	return strings.Join(msgs, "\n\n")
}

func (b *bot) ReceiveUpdate() (UpdateEvent, error) {
	for msg := range b.scWatcher.updateEvents {
		if msg.Message != "" {
			msg.Message = fmt.Sprintf("%v\n%v", MessageMarker, msg.Message)
		}
		return msg, nil
	}
	return UpdateEvent{}, io.EOF
}

func isOwnedBy(obj metav1.Object, owner metav1.Object) bool {
	for _, ownerRef := range obj.GetOwnerReferences() {
		if ownerRef.UID == owner.GetUID() {
			return true
		}
	}
	return false
}

func (k kubeClients) getSiteForSIS(sis *siteapi.SiteImageSource) (metav1.Object, error) {
	if sites, err := k.dLister.DrupalSites(sis.Namespace).List(labels.Everything()); err == nil {
		for _, site := range sites {
			if isOwnedBy(sis, site) {
				return site, nil
			}
		}
	}
	if sites, err := k.tLister.Typo3Sites(sis.Namespace).List(labels.Everything()); err == nil {
		for _, site := range sites {
			if isOwnedBy(sis, site) {
				return site, nil
			}
		}
	}
	if sites, err := k.wLister.Wordpresses(sis.Namespace).List(labels.Everything()); err == nil {
		for _, site := range sites {
			if isOwnedBy(sis, site) {
				return site, nil
			}
		}
	}
	return nil, fmt.Errorf("Site for SiteImageSource %v/%v not found", sis.Namespace, sis.Name)
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

func (k kubeClients) previewSiteUpdateFromSiteClone(sc *siteapi.SiteClone) (UpdateEvent, error) {
	if sc.Annotations[botAnnotation] != k.annotation {
		return UpdateEvent{}, nil
	}
	pr, err := strconv.Atoi(sc.Annotations[prAnnotation])
	if err != nil {
		return UpdateEvent{}, fmt.Errorf("failed to parse %q annotation: %v", prAnnotation, err)
	}

	ss, err := k.getSiteStatus(sc.Namespace, sc.Spec.Clone.Name)
	if err != nil {
		klog.V(2).Infof("Site %v/%v not found yet", sc.Namespace, sc.Spec.Clone.Name)
	}
	var bs buildStatus
	if c := common.GetCondition(ss.conditions, common.SiteImageSourceHealthy); c != nil && c.Reason == "Failed" {
		bs = k.getBuildState(sc.Namespace, sc.Spec.Clone.Name)
	} else {
		bs = buildStatus{}
	}
	msg := previewCreating(sc, ss, bs)
	ue := UpdateEvent{
		Message:     msg,
		PR:          pr,
		RepoURL:     sc.Annotations[repoAnnotation],
		Annotations: sc.Annotations,
	}
	return ue, nil
}

func (k kubeClients) previewSiteUpdateFromCMS(obj metav1.Object) (UpdateEvent, error) {
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
		return UpdateEvent{}, err
	}
	return k.previewSiteUpdateFromSiteClone(sc)
}

func getFirstFailedCont(pod *v1.Pod) string {
	exitCodes := make(map[string]int32)
	for _, c := range pod.Status.ContainerStatuses {
		if c.State.Terminated != nil {
			exitCodes[c.Name] = c.State.Terminated.ExitCode
		}
	}
	for _, c := range pod.Spec.Containers {
		if exitCodes[c.Name] != 0 {
			return c.Name
		}
	}
	return ""
}

func (k kubeClients) getFailureLog(pods []*v1.Pod) string {
	lastPod, ps := pods[0], pods[1:]
	for i, p := range ps {
		if p.CreationTimestamp.After(lastPod.CreationTimestamp.Time) {
			lastPod = ps[i]
		}
	}
	failedCont := getFirstFailedCont(lastPod)
	if failedCont == "" {
		return ""
	}
	opts := &v1.PodLogOptions{Container: failedCont, LimitBytes: &logLimitBytes}
	// NOTE: this is not cached, we should ensure to fetch the logs only when necessary
	req := k.coreClientSet.CoreV1().Pods(lastPod.Namespace).GetLogs(lastPod.Name, opts)
	podLogs, err := req.Stream()
	if err != nil {
		klog.Errorf("failed getting logs from %v/%v-%v: %v", lastPod.Namespace, lastPod.Name, failedCont, err)
		return siteBuildLogFetchFailed
	}
	defer podLogs.Close()
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, podLogs); err != nil {
		klog.Errorf("failed copying logs from %v/%v-%v: %v", lastPod.Namespace, lastPod.Name, failedCont, err)
		return siteBuildLogFetchFailed
	}
	return buf.String()
}

func (k kubeClients) getFailedBuildLogs(sis *siteapi.SiteImageSource) string {
	selector := labels.Set(map[string]string{siteapi.SiteLabel: sis.Name, "app.kubernetes.io/managed-by": "tekton-pipelines"})
	pods, err := k.podLister.Pods(sis.Namespace).List(selector.AsSelector())
	if err != nil {
		klog.Errorf("failed to fetch build pods for SiteImageSource %v/%v: %v", sis.Namespace, sis.Name, err)
		return siteBuildLogFetchFailed
	}
	if len(pods) == 0 {
		klog.Errorf("no build pods for SiteImageSource %v/%v", sis.Namespace, sis.Name)
		return noSiteBuilds
	}
	return k.getFailureLog(pods)
}

func (k kubeClients) getBuildState(namespace, name string) buildStatus {
	sis, err := k.sisLister.SiteImageSources(namespace).Get(name)
	if err != nil {
		klog.Errorf("failed to determine build state for SiteImageSource %v/%v: %v", namespace, name, err)
		return buildStatus{failed: false}
	}
	if failed, msg := sis.Failed(); failed {
		logs := k.getFailedBuildLogs(sis)
		return buildStatus{failed: true, failState: msg, logs: logs}
	}
	return buildStatus{failed: false}
}

func (k kubeClients) previewSiteUpdate(obj interface{}) (UpdateEvent, error) {
	<-k.wait
	switch o := obj.(type) {
	case *siteapi.SiteClone:
		return k.previewSiteUpdateFromSiteClone(o)
	case *drupalapi.DrupalSite:
		return k.previewSiteUpdateFromCMS(o)
	case *typo3api.Typo3Site:
		return k.previewSiteUpdateFromCMS(o)
	case *wordpressapi.Wordpress:
		return k.previewSiteUpdateFromCMS(o)
	default:
		return UpdateEvent{}, fmt.Errorf("unsupported type %T for preview site update", o)
	}
	return UpdateEvent{}, nil
}

func (k kubeClients) deletePreviewSiteUpdate(sc *siteapi.SiteClone) (string, int, error) {
	if sc.Annotations[botAnnotation] != k.annotation {
		return "", 0, nil
	}
	pr, err := strconv.Atoi(sc.Annotations[prAnnotation])
	if err != nil {
		return previewGenericError, 0, fmt.Errorf("failed to parse %q annotation: %v", prAnnotation, err)
	}
	return fmt.Sprintf(deletedSite, sc.Spec.Clone.Name, sc.Namespace), pr, nil
}
