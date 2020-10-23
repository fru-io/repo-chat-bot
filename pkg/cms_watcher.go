package pkg

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	drupalapi "github.com/drud/ddev-live-sdk/golang/pkg/drupal/apis/cms/v1beta1"
	typo3api "github.com/drud/ddev-live-sdk/golang/pkg/typo3/apis/cms/v1beta1"
	wordpressapi "github.com/drud/ddev-live-sdk/golang/pkg/wordpress/apis/cms/v1beta1"
)

type cmsWatcher struct {
	updateEvents chan UpdateEvent

	kubeClients
}

type statusCheck interface {
	Ready() (bool, string)
}

func (w cmsWatcher) OnAdd(obj interface{}) {
	meta := validMeta(obj)
	if meta == nil {
		return
	}
	w.enqeueueMsg(meta)
	return
}

func (w cmsWatcher) OnUpdate(oldObj, newObj interface{}) {
	meta := validMeta(newObj)
	if meta == nil {
		return
	}
	oldStatus, ok := oldObj.(statusCheck)
	if !ok {
		return
	}
	newStatus, ok := oldObj.(statusCheck)
	if !ok {
		return
	}
	or, os := oldStatus.Ready()
	nr, ns := newStatus.Ready()
	if or == nr && os == ns {
		return
	}
	w.enqeueueMsg(meta)
}

func (w cmsWatcher) OnDelete(obj interface{}) {
	return
}

func (w cmsWatcher) enqeueueMsg(obj metav1.Object) {
	if !obj.GetDeletionTimestamp().IsZero() {
		return
	}
	msg, pr, err := w.previewSiteUpdate(obj)
	if err != nil {
		klog.Errorf("dropping event for sc %v/%v: %v", obj.GetNamespace(), obj.GetName(), err)
		return
	}
	ue := UpdateEvent{
		Message:     msg,
		PR:          pr,
		RepoURL:     obj.GetAnnotations()[repoAnnotation],
		Type:        fmt.Sprintf("%TUpdate", obj),
		Annotations: obj.GetAnnotations(),
	}
	if len(w.updateEvents) == cap(w.updateEvents) {
		klog.Errorf("dropping event %v due to channel capacity: len(%v) == cap(%v)", ue, len(w.updateEvents), cap(w.updateEvents))
		return
	}
	w.updateEvents <- ue
}

func validMeta(obj interface{}) metav1.Object {
	switch obj.(type) {
	case drupalapi.DrupalSite, typo3api.Typo3Site, wordpressapi.Wordpress:
		meta, ok := obj.(metav1.ObjectMetaAccessor)
		if !ok {
			return nil
		}
		if !hasOwnerSiteClone(meta.GetObjectMeta()) {
			return nil
		}
		return meta.GetObjectMeta()
	}
	return nil
}

func hasOwnerSiteClone(obj metav1.Object) bool {
	for _, o := range obj.GetOwnerReferences() {
		if o.Kind == "SiteClone" {
			return true
		}
	}
	return false
}
