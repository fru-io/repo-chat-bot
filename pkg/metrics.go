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
	"k8s.io/klog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	segment "gopkg.in/segmentio/analytics-go.v3"
)

// Prometheus metrics for bot command invocations
var (
	CreatePreviewSiteCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bot_create_preview_site_total",
		Help: "Total number of create preview site command invocations",
	}, []string{"bot", "status"})

	DeletePreviewSiteCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bot_delete_preview_site_total",
		Help: "Total number of delete preview site command invocations",
	}, []string{"bot", "action", "status"})

	HelpPreviewSiteCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bot_help_preview_site_total",
		Help: "Total number of help command invocations",
	}, []string{"bot", "action"})

	// Label for counters that can be triggered either by command directly or action in a repository
	metricLabelCommand = "command"

	// Labels for site creation status
	metricLabelPreviewSuccess            = "success"
	metricLabelPreviewFailed             = "internal error"
	metricLabelPreviewDeniedMissingEmail = "failed to verify user email"
	metricLabelPreviewAuthAPIError       = "failed to query admin API"
	metricLabelPreviewDenied             = "unauthorized"
	metricLabelPreviewNothingToDelete    = "no preview site to be deleted"
)

type analytics struct {
	botType string
	appId   string
	cli     segment.Client
}

type noopLog struct{}

func (n *noopLog) Logf(f string, args ...interface{})   {}
func (n *noopLog) Errorf(f string, args ...interface{}) {}

func AnalyticsClient(botType, appId, writeKey string) *analytics {
	if writeKey == "" {
		klog.Info("analytics client not configured")
		return &analytics{
			botType: botType,
			appId:   appId,
		}
	}
	cli, err := segment.NewWithConfig(writeKey, segment.Config{Logger: &noopLog{}})
	if err != nil {
		klog.Errorf("failed to init analytics client: %v", err)
	}
	return &analytics{
		botType: botType,
		appId:   appId,
		cli:     cli,
	}
}

func (a *analytics) ObserveCreatePreviewSite(user, result string) {
	CreatePreviewSiteCounter.WithLabelValues(a.botType, result).Inc()
	if a.cli == nil {
		klog.V(9).Info("analytics client disabled")
		return
	}
	props := segment.NewProperties()
	props.Set("result", result)
	props.Set("type", metricLabelCommand)
	props.Set("bot", a.botType)
	msg := segment.Track{
		UserId:     user,
		Event:      PreviewSite,
		Properties: props,
		Context: &segment.Context{
			App: segment.AppInfo{
				Name: a.appId,
			},
		},
	}
	if err := a.cli.Enqueue(msg); err != nil {
		klog.Errorf("failed to send analytics for create preview site: %v", err)
	}
}

func (a *analytics) ObserveDeletePreviewSite(user, t, result string) {
	DeletePreviewSiteCounter.WithLabelValues(a.botType, t, result).Inc()
	if a.cli == nil {
		klog.V(9).Info("analytics client disabled")
		return
	}
	props := segment.NewProperties()
	props.Set("result", result)
	props.Set("type", t)
	props.Set("bot", a.botType)
	msg := segment.Track{
		UserId:     user,
		Event:      DeletePreviewSite,
		Properties: props,
		Context: &segment.Context{
			App: segment.AppInfo{
				Name: a.appId,
			},
		},
	}
	if err := a.cli.Enqueue(msg); err != nil {
		klog.Errorf("failed to send analytics for delete preview site: %v", err)
	}
}

func (a *analytics) ObserveHelp(user, t string) {
	HelpPreviewSiteCounter.WithLabelValues(a.botType, t).Inc()
	if a.cli == nil {
		klog.V(9).Info("analytics client disabled")
		return
	}
	props := segment.NewProperties()
	props.Set("result", "success")
	props.Set("type", t)
	props.Set("bot", a.botType)
	msg := segment.Track{
		UserId:     user,
		Event:      Help,
		Properties: props,
		Context: &segment.Context{
			App: segment.AppInfo{
				Name: a.appId,
			},
		},
	}
	if err := a.cli.Enqueue(msg); err != nil {
		klog.Errorf("failed to send analytics for help preview site: %v", err)
	}
}
