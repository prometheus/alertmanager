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
              severity: 'critical',
            },
            annotations: {
              summary: 'Reloading an Alertmanager configuration has failed.',
              description: 'Configuration has failed to load for %(alertmanagerName)s.' % $._config,
            },
          },
          {
            alert: 'AlertmanagerMembersInconsistent',
            expr: |||
              # Without max_over_time, failed scrapes could create false negatives, see
              # https://www.robustperception.io/alerting-on-gauges-in-prometheus-2-0 for details.
                max_over_time(alertmanager_cluster_members{%(alertmanagerSelector)s}[5m])
              < on (job) group_left
                count by (job) (max_over_time(alertmanager_cluster_members{%(alertmanagerSelector)s}[5m]))
            ||| % $._config,
            'for': '10m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              summary: 'A member of an Alertmanager cluster has not found all other cluster members.',
              description: 'Alertmanager %(alertmanagerName)s has only found {{ $value }} members of the {{ $labels.job }} cluster.' % $._config,
            },
          },
          {
            alert: 'AlertmanagerFailedToSendAlerts',
            expr: |||
              (
                rate(alertmanager_notifications_failed_total{%(alertmanagerSelector)s}[5m])
              /
                rate(alertmanager_notifications_total{%(alertmanagerSelector)s}[5m])
              )
              > 0.01
            ||| % $._config,
            'for': '5m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              summary: 'An Alertmanager instance fails to send notifications.',
              description: 'Alertmanager %(alertmanagerName)s failed to send {{ $value | humanizePercentage }}%% notifications to {{ $labels.integration }}.' % $._config,
            },
          },
        ],
      },
    ],
  },
}
