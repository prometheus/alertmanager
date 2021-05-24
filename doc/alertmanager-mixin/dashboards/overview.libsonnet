local grafana = import 'github.com/grafana/grafonnet-lib/grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local row = grafana.row;
local prometheus = grafana.prometheus;
local template = grafana.template;
local graphPanel = grafana.graphPanel;

{
  grafanaDashboards+:: {
    local alertmanagerClusterSelectorTemplates =
      [
        template.new(
          name=label,
          datasource='$datasource',
          query='label_values(alertmanager_alerts, %s)' % label,
          current='',
          refresh=2,
          includeAll=false,
          sort=1
        )
        for label in std.split($._config.alertmanagerClusterLabels, ',')
      ],

    local integrationTemplate =
      template.new(
        name='integration',
        datasource='$datasource',
        query='label_values(alertmanager_notifications_total{integration=~"%s"}, integration)' % $._config.alertmanagerCriticalIntegrationsRegEx,
        current='all',
        hide='2',  // Always hide
        refresh=2,
        includeAll=true,
        sort=1
      ),

    'alertmanager-overview.json':
      local alerts =
        graphPanel.new(
          'Alerts',
          datasource='$datasource',
          span=6,
          format='none',
          stack=true,
          fill=1,
          legend_show=false,
        )
        .addTarget(prometheus.target('sum(alertmanager_alerts{%(alertmanagerQuerySelector)s}) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config, legendFormat='%(alertmanagerNameDashboards)s' % $._config));

      local alertsRate =
        graphPanel.new(
          'Alerts receive rate',
          datasource='$datasource',
          span=6,
          format='ops',
          stack=true,
          fill=1,
          legend_show=false,
        )
        .addTarget(prometheus.target('sum(rate(alertmanager_alerts_received_total{%(alertmanagerQuerySelector)s}[5m])) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config, legendFormat='%(alertmanagerNameDashboards)s Received' % $._config))
        .addTarget(prometheus.target('sum(rate(alertmanager_alerts_invalid_total{%(alertmanagerQuerySelector)s}[5m])) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config, legendFormat='%(alertmanagerNameDashboards)s Invalid' % $._config));

      local notifications =
        graphPanel.new(
          '$integration: Notifications Send Rate',
          datasource='$datasource',
          format='ops',
          stack=true,
          fill=1,
          legend_show=false,
          repeat='integration'
        )
        .addTarget(prometheus.target('sum(rate(alertmanager_notifications_total{%(alertmanagerQuerySelector)s, integration="$integration"}[5m])) by (integration,%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config, legendFormat='%(alertmanagerNameDashboards)s Total' % $._config))
        .addTarget(prometheus.target('sum(rate(alertmanager_notifications_failed_total{%(alertmanagerQuerySelector)s, integration="$integration"}[5m])) by (integration,%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config, legendFormat='%(alertmanagerNameDashboards)s Failed' % $._config));

      local notificationDuration =
        graphPanel.new(
          '$integration: Notification Duration',
          datasource='$datasource',
          format='s',
          stack=false,
          fill=1,
          legend_show=false,
          repeat='integration'
        )
        .addTarget(prometheus.target(
          |||
            histogram_quantile(0.99,
              sum(rate(alertmanager_notification_latency_seconds_bucket{%(alertmanagerQuerySelector)s, integration="$integration"}[5m])) by (le,%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)
            ) 
          ||| % $._config, legendFormat='%(alertmanagerNameDashboards)s 99th Percentile' % $._config
        ))
        .addTarget(prometheus.target(
          |||
            histogram_quantile(0.50,
              sum(rate(alertmanager_notification_latency_seconds_bucket{%(alertmanagerQuerySelector)s, integration="$integration"}[5m])) by (le,%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)
            ) 
          ||| % $._config, legendFormat='%(alertmanagerNameDashboards)s Median' % $._config
        ))
        .addTarget(prometheus.target(
          |||
            sum(rate(alertmanager_notification_latency_seconds_sum{%(alertmanagerQuerySelector)s, integration="$integration"}[5m])) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)
            /
            sum(rate(alertmanager_notification_latency_seconds_count{%(alertmanagerQuerySelector)s, integration="$integration"}[5m])) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)
          ||| % $._config, legendFormat='%(alertmanagerNameDashboards)s Average' % $._config
        ));

      dashboard.new(
        '%sOverview' % $._config.dashboardNamePrefix,
        time_from='now-1h',
        tags=($._config.dashboardTags),
        timezone='utc',
        refresh='30s',
        graphTooltip='shared_crosshair',
        uid='alertmanager-overview'
      )
      .addTemplate(
        {
          current: {
            text: 'Prometheus',
            value: 'Prometheus',
          },
          hide: 0,
          label: null,
          name: 'datasource',
          options: [],
          query: 'prometheus',
          refresh: 1,
          regex: '',
          type: 'datasource',
        },
      )
      .addTemplates(alertmanagerClusterSelectorTemplates)
      .addTemplate(integrationTemplate)
      .addRow(
        row.new('Alerts')
        .addPanel(alerts)
        .addPanel(alertsRate)
      )
      .addRow(
        row.new('Notifications')
        .addPanel(notifications)
        .addPanel(notificationDuration)
      ),
  },
}
