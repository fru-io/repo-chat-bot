module github.com/drud/repo-chat-bot

go 1.15

require (
	github.com/drud/admin-api v0.4.2
	github.com/drud/cms-common v0.0.7
	github.com/drud/ddev-live-sdk v0.2.8
	github.com/prometheus/client_golang v1.7.1
	github.com/whilp/git-urls v1.0.0
	google.golang.org/grpc v1.29.1
	k8s.io/api v0.17.9
	k8s.io/apimachinery v0.18.5
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/klog v1.0.0
)

replace (
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.9
	k8s.io/client-go => k8s.io/client-go v0.17.9
)
