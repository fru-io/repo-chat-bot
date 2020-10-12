package pkg

import (
	drupallister "github.com/drud/ddev-live-sdk/golang/pkg/drupal/listers/cms/v1beta1"
	sitelister "github.com/drud/ddev-live-sdk/golang/pkg/site/listers/site/v1beta1"
)

type cmsWatcher struct {
	annotation   string
	scLister     sitelister.SiteCloneLister
	dLister      drupallister.DrupalSiteLister
	updateEvents chan UpdateEvent
}

func (w cmsWatcher) OnAdd(obj interface{}) {
	return
}

func (w cmsWatcher) OnUpdate(oldObj, newObj interface{}) {
	return
}

func (w cmsWatcher) OnDelete(obj interface{}) {
	return
}
