{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'alertmanager.rules',
        rules: [
          {
            alert: 'AlertmanagerFailedReload',
            expr: |||
              alertmanager_config_last_reload_successful{%(alertmanagerSelector)s} == 0
            ||| % $._config,
            'for': '10m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: "Reloading Alertmanager's configuration has failed for {{ $labels.namespace }}/{{ $labels.pod}}.",
            },
          },
          {
            alert:'AlertmanagerMembersInconsistent',
            expr: |||
              alertmanager_cluster_members{%(alertmanagerSelector)s}
                != on (service)
              count by (service) (alertmanager_cluster_members{%(alertmanagerSelector)s})
            ||| % $._config,
            'for': '5m',
            labels: {
              severity: 'critical',
            },
            annotations:{
              message: 'Alertmanager has not found all other members of the cluster.',
            },
          },
          {
            alert: 'AlertmanagerFailedToSendAlerts',
            expr: |||
              100 * rate(alertmanager_notifications_failed_total{%(alertmanagerSelector)s}[5m])
              /
              rate(alertmanager_notifications_total{%(alertmanagerSelector)s}[5m]) > 1
            ||| % $._config,
            'for': '5m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'Alertmanager failed to send {{ printf "%.1f" $value }}% alerts to {{ $labels.integration }}.',
            },
          },
        ],
      },
    ],
  },
}
