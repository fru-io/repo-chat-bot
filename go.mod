module github.com/drud/repo-chat-bot

go 1.15

require (
	github.com/bmizerany/assert v0.0.0-20160611221934-b7ed37b82869 // indirect
	github.com/drud/admin-api v0.4.2
	github.com/drud/cms-common v0.0.10
	github.com/drud/ddev-live-sdk v0.3.2
	github.com/prometheus/client_golang v1.7.1
	github.com/segmentio/backo-go v0.0.0-20160424052352-204274ad699c // indirect
	github.com/whilp/git-urls v1.0.0
	github.com/xtgo/uuid v0.0.0-20140804021211-a0b114877d4c // indirect
	google.golang.org/grpc v1.29.1
	gopkg.in/segmentio/analytics-go.v3 v3.0.1
	k8s.io/api v0.17.9
	k8s.io/apimachinery v0.18.5
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/klog v1.0.0
)

replace (
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.9
	k8s.io/client-go => k8s.io/client-go v0.17.9
)
