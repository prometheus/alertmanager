# Alertmanager Mixin

The Alertmanager Mixin is a set of configurable, reusable and extensible alerts
(and eventually dashboards) for Alertmanager.

The alerts are designed to monitor a cluster of Alertmanager instances. To make
them work as expected, the Prometheus server the alerts are evaluated on has to
scrape all Alertmanager instances of the cluster, even if those instances are
distributed over different locations. All Alertmanager instances in the same
Alertmanager cluster must have the same `job` label. In turn, if monitoring
multiple different Alertmanager clusters, instances from different clusters
must have a different `job` label.

To use them, you need to have `jsonnet` (v0.13+) and `jb` installed. If you
have a working Go development environment, it's easiest to run the following:

```bash
$ go get github.com/google/go-jsonnet/cmd/jsonnet
$ go get github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb
```

_Note: The make targets `lint` and `fmt` need the `jsonnetfmt` binary, which is
currently not included in the Go implementation of `jsonnet`. For the time
being, you have to install the [C++ version of
jsonnetfmt](https://github.com/google/jsonnet) if you want to use `make lint`
or `make fmt`._

Next, install the dependencies by running the following command in this
directory:

```bash
$ jb install
```

You can then build a `alertmanager_alerts.yaml` with the alerts:

```bash
$ make alertmanager_alerts.yaml
```

For instructions on how to use mixins, see https://github.com/monitoring-mixins/docs.
