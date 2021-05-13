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
        .addTarget(prometheus.target('alertmanager_alerts{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}' % $._config, legendFormat='{{ state }}'));

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
          'Rate',
          datasource='$datasource',
          span=4,
          format='ops',
          stack=false,
          fill=1,
          legend_show=true,
        )
        .addTarget(prometheus.target('sum(rate(alertmanager_notifications_total{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])) by (integration)' % $._config, legendFormat='{{integration}}'));

      local notificationErrors =
        graphPanel.new(
          'Error',
          datasource='$datasource',
          span=4,
          format='ops',
          stack=false,
          fill=1,
          legend_show=true,
        )
        .addTarget(prometheus.target('sum(rate(alertmanager_notifications_failed_total{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])) by (integration)' % $._config, legendFormat='{{integration}}'));

      local notificationDuration =
        graphPanel.new(
          'Duration',
          datasource='$datasource',
          span=4,
          format='s',
          stack=false,
          fill=1,
          legend_show=true,
        )
        .addTarget(prometheus.target(
          |||
            histogram_quantile(0.99,
              rate(alertmanager_notification_latency_seconds_bucket{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])
            ) 
          ||| % $._config, legendFormat='{{integration}} - 99th Percentile'
        ))
        .addTarget(prometheus.target(
          |||
            histogram_quantile(0.50,
              rate(alertmanager_notification_latency_seconds_bucket{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])
            ) 
          ||| % $._config, legendFormat='{{integration}} - Median'
        ))
        .addTarget(prometheus.target(
          |||
            rate(alertmanager_notification_latency_seconds_sum{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])
            /
            rate(alertmanager_notification_latency_seconds_count{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])
          ||| % $._config, legendFormat='{{integration}} - Average'
        ));

      local silences =
        graphPanel.new(
          'Silences',
          datasource='$datasource',
          span=4,
          format='none',
          stack=false,
          fill=1,
          legend_show=false,
        )
        .addTarget(prometheus.target('sum(alertmanager_silences{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}) by (state)' % $._config, legendFormat='{{ state }}'));

      local silenceQueries =
        graphPanel.new(
          'Queries',
          datasource='$datasource',
          span=4,
          format='reqps',
          stack=false,
          fill=1,
          legend_show=false,
        )
        .addTarget(prometheus.target('rate(alertmanager_silences_queries_total{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])' % $._config, legendFormat='Queries'))
        .addTarget(prometheus.target('rate(alertmanager_silences_query_errors_total{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])' % $._config, legendFormat='Errors'));

      local silenceQueryDuration =
        graphPanel.new(
          'Queries Duration',
          datasource='$datasource',
          span=4,
          format='s',
          stack=false,
          fill=1,
          legend_show=true,
        )
        .addTarget(prometheus.target(
          |||
            histogram_quantile(0.99,
              rate(alertmanager_silences_query_duration_seconds_bucket{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])
            ) 
          ||| % $._config, legendFormat='99th Percentile'
        ))
        .addTarget(prometheus.target(
          |||
            histogram_quantile(0.50,
              rate(alertmanager_silences_query_duration_seconds_bucket{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])
            ) 
          ||| % $._config, legendFormat='Median'
        ))
        .addTarget(prometheus.target(
          |||
            rate(alertmanager_silences_query_duration_seconds_sum{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])
            /
            rate(alertmanager_silences_query_duration_seconds_count{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])
          ||| % $._config, legendFormat='Average'
        ));

      local silencesGossips =
        graphPanel.new(
          'Gossips',
          datasource='$datasource',
          span=4,
          format='reqps',
          stack=false,
          fill=1,
          legend_show=false,
        )
        .addTarget(prometheus.target('rate(alertmanager_silences_gossip_messages_propagated_total{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])' % $._config, legendFormat='Messages propagated'));

      local silenceGcDuration =
        graphPanel.new(
          'Garbage Collection Duration',
          datasource='$datasource',
          span=4,
          format='s',
          stack=false,
          fill=1,
          legend_show=false,
        )
        .addTarget(prometheus.target(
          |||
            rate(alertmanager_silences_gc_duration_seconds_sum{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])
            /
            rate(alertmanager_silences_gc_duration_seconds_count{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])
          ||| % $._config, legendFormat='Duration'
        ));

      local silenceSnaphotDuration =
        graphPanel.new(
          'Snapshot Duration',
          datasource='$datasource',
          span=4,
          format='s',
          stack=false,
          fill=1,
          legend_show=false,
        )
        .addTarget(prometheus.target(
          |||
            rate(alertmanager_silences_snapshot_duration_seconds_sum{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])
            /
            rate(alertmanager_silences_snapshot_duration_seconds_count{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])
          ||| % $._config, legendFormat='Duration'
        ));

      local HTTPRequestRate =
        graphPanel.new(
          'Request Rate',
          datasource='$datasource',
          span=6,
          format='reqps',
          stack=false,
          fill=1,
          legend_show=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('sum(rate(alertmanager_http_request_duration_seconds_count{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])) by (handler)' % $._config, legendFormat='{{handler}}'));

      local HTTPRequestDuration =
        graphPanel.new(
          'Request Duration',
          datasource='$datasource',
          span=6,
          format='s',
          stack=false,
          fill=1,
          legend_show=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target(
          |||
            histogram_quantile(0.99,
              rate(alertmanager_http_request_duration_seconds_bucket{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])
            ) 
          ||| % $._config, legendFormat='{{handler}} - 99th Percentile'
        ))
        .addTarget(prometheus.target(
          |||
            histogram_quantile(0.50,
              rate(alertmanager_http_request_duration_seconds_bucket{%(alertmanagerSelector)s, %(clusterLabel)s="$cluster"}[5m])
            ) 
          ||| % $._config, legendFormat='{{handler}} - 50th Percentile'
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
      .addRow(
        row.new('Alerts')
        .addPanel(alerts)
        .addPanel(alertsRate)
      )
      .addRow(
        row.new('Notifications')
        .addPanel(notifications)
        .addPanel(notificationErrors)
        .addPanel(notificationDuration)
      )
      .addRow(
        row.new('Silences')
        .addPanel(silences)
        .addPanel(silenceQueries)
        .addPanel(silenceQueryDuration)
        .addPanel(silencesGossips)
        .addPanel(silenceGcDuration)
        .addPanel(silenceSnaphotDuration)
      )
      .addRow(
        row.new('HTTP')
        .addPanel(HTTPRequestRate)
        .addPanel(HTTPRequestDuration)
      ),
  },
}
