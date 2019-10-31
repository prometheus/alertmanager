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
	    // TODO(beorn7): This alert is problematic for Alertmanager clusters
	    // spanning regions because the cross-regional Alertmanager clusters
	    // has more members than what the local Prometheus can see. In that
	    // case, local Alertmanagers not able to join the cluster won't be
	    // detected by this alert.
            alert:'AlertmanagerMembersInconsistent',
            expr: |||
              alertmanager_cluster_members{%(alertmanagerSelector)s}
                < on (job) group_left
              count by (job) (alertmanager_cluster_members{%(alertmanagerSelector)s})
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
