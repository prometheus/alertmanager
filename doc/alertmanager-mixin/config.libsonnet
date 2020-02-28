{
  _config+:: {
    // alertmanagerSelector is inserted as part of the label selector in
    // PromQL queries to identify metrics collected from Alertmanager
    // servers.
    alertmanagerSelector: 'job="alertmanager"',

    // alertmanagerName is inserted into annotations to name the Alertmanager
    // instance affected by the alert.
    alertmanagerName: '{{$labels.instance}}',
    // If you run Alertmanager on Kubernetes with the Prometheus
    // Operator, you can make use of the configured target labels for
    // nicer naming:
    // alertmanagerName: '{{$labels.namespace}}/{{$labels.pod}}'
  },
}
