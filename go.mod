module github.com/ghetzel/cloudflare-operator

go 1.13

require (
	github.com/cloudflare/cloudflare-go v0.12.0
	github.com/ghetzel/go-stockutil v1.8.72
	github.com/go-logr/logr v0.1.0
	github.com/operator-framework/operator-sdk v0.18.2
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.18.2
	k8s.io/apimachinery v0.18.2
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.6.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.18.2 // Required by prometheus-operator
)
