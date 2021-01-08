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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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
)
