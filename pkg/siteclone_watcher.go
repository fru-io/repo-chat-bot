package pkg

import (
	siteapi "github.com/drud/ddev-live-sdk/golang/pkg/site/apis/site/v1beta1"
	"k8s.io/klog"
)

type scWatcher struct {
	updateEvents chan UpdateEvent

	kubeClients
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
	o, ok := obj.(*siteapi.SiteClone)
	if !ok {
		return
	}
	msg, pr, err := w.deletePreviewSiteUpdate(o)
	if err != nil {
		klog.Errorf("dropping event for sc %v/%v: %v", o.GetNamespace(), o.GetName(), err)
		return
	}
	if msg == "" {
		klog.Errorf("dropping event for sc %v/%v: empty response message", o.GetNamespace(), o.GetName())
		return
	}
	ue := UpdateEvent{
		Message:     msg,
		PR:          pr,
		RepoURL:     o.GetAnnotations()[repoAnnotation],
		Type:        "SiteCloneDelete",
		Annotations: o.GetAnnotations(),
	}
	if len(w.updateEvents) == cap(w.updateEvents) {
		klog.Errorf("dropping event %v due to channel capacity: len(%v) == cap(%v)", ue, len(w.updateEvents), cap(w.updateEvents))
		return
	}
	w.updateEvents <- ue
}

func (w scWatcher) enqueueMsg(sc *siteapi.SiteClone) {
	if !sc.DeletionTimestamp.IsZero() {
		return
	}
	if sc.Annotations == nil || sc.Annotations[botAnnotation] != w.kubeClients.annotation {
		return
	}
	ue, err := w.previewSiteUpdate(sc)
	if err != nil {
		klog.Errorf("dropping event for sc %v/%v: %v", sc.Namespace, sc.Name, err)
		return
	}
	ue.Type = "SiteCloneUpdate"
	if len(w.updateEvents) == cap(w.updateEvents) {
		klog.Errorf("dropping event %v due to channel capacity: len(%v) == cap(%v)", ue, len(w.updateEvents), cap(w.updateEvents))
		return
	}
	w.updateEvents <- ue
}
