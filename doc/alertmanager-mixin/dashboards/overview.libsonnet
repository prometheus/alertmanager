local grafana = import 'github.com/grafana/grafonnet-lib/grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local row = grafana.row;
local prometheus = grafana.prometheus;
local template = grafana.template;
local graphPanel = grafana.graphPanel;

{
  grafanaDashboards+:: {
    local clusterTemplate =
      template.new(
        name='cluster',
        datasource='$datasource',
        query='label_values(alertmanager_alerts, %s)' % $._config.clusterLabel,
        current='',
        hide=if $._config.showMultiCluster then '' else '2',
        refresh=2,
        includeAll=false,
        sort=1
      ),

    local integrationTemplate =
      template.new(
        name='integration',
        datasource='$datasource',
        query='label_values(alertmanager_notifications_total{integration=~"%s"}, integration)' % $._config.alertmanagerCriticalIntegrationsRegEx,
        current='all',
        hide='2', # Always hide
        refresh=2,
        includeAll=true,
        sort=1
      ),

    local legendFormatPrefix =
      (if $._config.showMultiCluster then '{{ %(clusterLabel)s }}/' % $._config else ''),

    'alertmanager-overview.json':
      local alerts =
        graphPanel.new(
          'Alerts',
          datasource='$datasource',
          span=6,
          format='none',
          stack=false,
          fill=1,
          legend_show=false,
        )
        .addTarget(prometheus.target('alertmanager_alerts{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}' % $._config, legendFormat=legendFormatPrefix + '%(alertmanagerName)s - {{ state }}' % $._config));

      local alertsRate =
        graphPanel.new(
          'Alerts receive rate',
          datasource='$datasource',
          span=6,
          format='ops',
          stack=false,
          fill=1,
          legend_show=false,
        )
        .addTarget(prometheus.target('sum(rate(alertmanager_alerts_received_total{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m]))' % $._config, legendFormat='Received'))
        .addTarget(prometheus.target('sum(rate(alertmanager_alerts_invalid_total{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m]))' % $._config, legendFormat='Invalid'));

      local notifications =
        graphPanel.new(
          '$integration: Notifications Send Rate',
          datasource='$datasource',
          format='ops',
          stack=false,
          fill=1,
          legend_show=false,
          repeat='integration'
        )
        .addTarget(prometheus.target('sum(rate(alertmanager_notifications_total{%(alertmanagerSelector)s, integration="$integration", %(clusterLabel)s="$cluster"}[5m])) by (integration)' % $._config, legendFormat='Total'))
        .addTarget(prometheus.target('sum(rate(alertmanager_notifications_failed_total{%(alertmanagerSelector)s, integration="$integration", %(clusterLabel)s="$cluster"}[5m])) by (integration)' % $._config, legendFormat='Failed'));

      local notificationDuration =
        graphPanel.new(
          '$integration: Notification Duration',
          datasource='$datasource',
          format='s',
          stack=false,
          fill=1,
          legend_show=true,
          repeat='integration'
        )
        .addTarget(prometheus.target(
          |||
            histogram_quantile(0.99,
              rate(alertmanager_notification_latency_seconds_bucket{%(alertmanagerSelector)s, integration="$integration", %(clusterLabel)s="$cluster"}[5m])
            ) 
          ||| % $._config, legendFormat='99th Percentile'
        ))
        .addTarget(prometheus.target(
          |||
            histogram_quantile(0.50,
              rate(alertmanager_notification_latency_seconds_bucket{%(alertmanagerSelector)s, integration="$integration", %(clusterLabel)s="$cluster"}[5m])
            ) 
          ||| % $._config, legendFormat='Median'
        ))
        .addTarget(prometheus.target(
          |||
            rate(alertmanager_notification_latency_seconds_sum{%(alertmanagerSelector)s, integration="$integration", %(clusterLabel)s="$cluster"}[5m])
            /
            rate(alertmanager_notification_latency_seconds_count{%(alertmanagerSelector)s, integration="$integration", %(clusterLabel)s="$cluster"}[5m])
          ||| % $._config, legendFormat='Average'
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
      .addTemplate(clusterTemplate)
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
