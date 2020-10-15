package pkg

import (
	"k8s.io/klog"
	"strconv"

	siteapi "github.com/drud/ddev-live-sdk/golang/pkg/site/apis/site/v1beta1"
	sitelister "github.com/drud/ddev-live-sdk/golang/pkg/site/listers/site/v1beta1"
)

type scWatcher struct {
	annotation   string
	scLister     sitelister.SiteCloneLister
	updateEvents chan UpdateEvent
}

func (w scWatcher) OnAdd(obj interface{}) {
	o, ok := obj.(*siteapi.SiteClone)
	if !ok {
		return
	}
	w.enqueueMsg(o)
}

func (w scWatcher) OnUpdate(oldObj, newObj interface{}) {
	n, ok := oldObj.(*siteapi.SiteClone)
	if !ok {
		return
	}
	o, ok := newObj.(*siteapi.SiteClone)
	if !ok {
		return
	}
	nr, nm := n.Ready()
	or, om := o.Ready()
	if nr == or && nm == om {
		return
	}
	w.enqueueMsg(o)
}

func (w scWatcher) OnDelete(obj interface{}) {
	return
}

func (w scWatcher) enqueueMsg(sc *siteapi.SiteClone) {
	if sc.Annotations == nil || sc.Annotations[botAnnotation] != w.annotation {
		return
	}
	pr, err := strconv.Atoi(sc.Annotations[prAnnotation])
	if err != nil {
		klog.Errorf("dropping event for sc %v/%v, failed to convert PR annotation: %v", sc.Namespace, sc.Name, err)
		return
	}
	msg := previewCreating(sc)
	ue := UpdateEvent{
		Message:     msg,
		PR:          pr,
		RepoURL:     sc.Annotations[repoAnnotation],
		Type:        "SiteCloneUpdate",
		Annotations: sc.Annotations,
	}
	if len(w.updateEvents) == cap(w.updateEvents) {
		klog.Errorf("dropping event %v due to channel capacity: len(%v) == cap(%v)", ue, len(w.updateEvents), cap(w.updateEvents))
		return
	}
	w.updateEvents <- ue
}
