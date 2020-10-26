{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'alertmanager.rules',
        rules: [
          {
            alert: 'AlertmanagerFailedReload',
            expr: |||
              # Without max_over_time, failed scrapes could create false negatives, see
              # https://www.robustperception.io/alerting-on-gauges-in-prometheus-2-0 for details.
              max_over_time(alertmanager_config_last_reload_successful{%(alertmanagerSelector)s}[5m]) == 0
            ||| % $._config,
            'for': '10m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: "Reloading Alertmanager's configuration has failed for %(alertmanagerName)s" % $._config,
            },
          },
          {
            alert: 'AlertmanagerMembersInconsistent',
            expr: |||
              alertmanager_cluster_members{%(alertmanagerSelector)s}
                < on (job) group_left
              count by (job) (alertmanager_cluster_members{%(alertmanagerSelector)s})
            ||| % $._config,
            'for': '5m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'Alertmanager %(alertmanagerName)s has not found all other members of the cluster.' % $._config,
            },
          },
          {
            alert: 'AlertmanagerFailedToSendAlerts',
            expr: |||
              rate(alertmanager_notifications_failed_total{%(alertmanagerSelector)s}[5m])
              /
              rate(alertmanager_notifications_total{%(alertmanagerSelector)s}[5m]) > 0.01
            ||| % $._config,
            'for': '5m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'Alertmanager %(alertmanagerName)s failed to send {{ $value | humanizePercentage }} notifications to {{ $labels.integration }}.' % $._config,
            },
          },
        ],
      },
    ],
  },
}
