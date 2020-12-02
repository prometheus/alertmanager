module github.com/prometheus/alertmanager/doc/alertmanager-mixin

go 1.14

replace (
	k8s.io/klog => github.com/simonpasquier/klog-gokit v0.3.0
	k8s.io/klog/v2 => github.com/simonpasquier/klog-gokit/v2 v2.0.1
)

exclude (
	// Exclude grpc v1.30.0 because of breaking changes. See #7621.
	github.com/grpc-ecosystem/grpc-gateway v1.14.7
	google.golang.org/api v0.30.0

	// Exclude pre-go-mod kubernetes tags, as they are older
	// than v0.x releases but are picked when we update the dependencies.
	k8s.io/client-go v1.4.0
	k8s.io/client-go v1.4.0+incompatible
	k8s.io/client-go v1.5.0
	k8s.io/client-go v1.5.0+incompatible
	k8s.io/client-go v1.5.1
	k8s.io/client-go v1.5.1+incompatible
	k8s.io/client-go v10.0.0+incompatible
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/client-go v2.0.0+incompatible
	k8s.io/client-go v2.0.0-alpha.1+incompatible
	k8s.io/client-go v3.0.0+incompatible
	k8s.io/client-go v3.0.0-beta.0+incompatible
	k8s.io/client-go v4.0.0+incompatible
	k8s.io/client-go v4.0.0-beta.0+incompatible
	k8s.io/client-go v5.0.0+incompatible
	k8s.io/client-go v5.0.1+incompatible
	k8s.io/client-go v6.0.0+incompatible
	k8s.io/client-go v7.0.0+incompatible
	k8s.io/client-go v8.0.0+incompatible
	k8s.io/client-go v9.0.0+incompatible
	k8s.io/client-go v9.0.0-invalid+incompatible
)

require (
	github.com/Azure/azure-sdk-for-go v48.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.11 // indirect
	github.com/HdrHistogram/hdrhistogram-go v0.9.0 // indirect
	github.com/aws/aws-sdk-go v1.35.31 // indirect
	github.com/dgryski/go-sip13 v0.0.0-20200911182023-62edffca9245 // indirect
	github.com/digitalocean/godo v1.52.0 // indirect
	github.com/go-openapi/validate v0.19.14 // indirect
	github.com/golang/snappy v0.0.2 // indirect
	github.com/google/go-jsonnet v0.17.0 // indirect
	github.com/google/pprof v0.0.0-20201117184057-ae444373da19 // indirect
	github.com/gophercloud/gophercloud v0.14.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/hashicorp/consul/api v1.7.0 // indirect
	github.com/hetznercloud/hcloud-go v1.23.1 // indirect
	github.com/influxdata/influxdb v1.8.3 // indirect
	github.com/miekg/dns v1.1.35 // indirect
	github.com/monitoring-mixins/mixtool v0.0.0-20201127170310-63dca667103c // indirect
	github.com/prometheus/client_golang v1.8.0 // indirect
	github.com/prometheus/common v0.15.0 // indirect
	github.com/shurcooL/vfsgen v0.0.0-20200824052919-0d455de96546 // indirect
	github.com/uber/jaeger-lib v2.4.0+incompatible // indirect
	go.uber.org/atomic v1.7.0 // indirect
	golang.org/x/oauth2 v0.0.0-20201109201403-9fd604954f58 // indirect
	golang.org/x/sys v0.0.0-20201119102817-f84b799fce68 // indirect
	golang.org/x/tools v0.0.0-20201119054027-25dc3e1ccc3c // indirect
	google.golang.org/api v0.35.0 // indirect
	k8s.io/client-go v0.19.4 // indirect
	k8s.io/klog/v2 v2.4.0 // indirect
)
