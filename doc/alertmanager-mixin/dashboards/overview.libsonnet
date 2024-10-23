local grafana = import 'github.com/grafana/grafonnet/gen/grafonnet-latest/main.libsonnet';
local dashboard = grafana.dashboard;
local prometheus = grafana.query.prometheus;
local variable = dashboard.variable;
local panel = grafana.panel;
local row = panel.row;

{
  grafanaDashboards+:: {
    local amQuerySelector = std.join(',', ['%s=~"$%s"' % [label, label] for label in std.split($._config.alertmanagerClusterLabels, ',')]),
    local amNameDashboardLegend = std.join('/', ['{{%s}}' % [label] for label in std.split($._config.alertmanagerNameLabels, ',')]),

    local datasource =
      variable.datasource.new('datasource', 'prometheus')
      + variable.datasource.generalOptions.withLabel('Data Source')
      + variable.datasource.generalOptions.withCurrent('Prometheus')
      + variable.datasource.generalOptions.showOnDashboard.withLabelAndValue(),

    local alertmanagerClusterSelectorVariables =
      [
        variable.query.new(label)
        + variable.query.generalOptions.withLabel(label)
        + variable.query.withDatasourceFromVariable(datasource)
        + variable.query.queryTypes.withLabelValues(label, metric='alertmanager_alerts')
        + variable.query.generalOptions.withCurrent('')
        + variable.query.refresh.onTime()
        + variable.query.selectionOptions.withIncludeAll(false)
        + variable.query.withSort(type='alphabetical')
        for label in std.split($._config.alertmanagerClusterLabels, ',')
      ],

    local integrationVariable =
      variable.query.new('integration')
      + variable.query.withDatasourceFromVariable(datasource)
      + variable.query.queryTypes.withLabelValues('integration', metric='alertmanager_notifications_total{integration=~"%s"}' % $._config.alertmanagerCriticalIntegrationsRegEx)
      + variable.query.generalOptions.withCurrent('$__all')
      + variable.datasource.generalOptions.showOnDashboard.withNothing()
      + variable.query.refresh.onTime()
      + variable.query.selectionOptions.withIncludeAll(true)
      + variable.query.withSort(type='alphabetical'),

    local panelTimeSeriesStdOptions =
      {}
      + panel.timeSeries.fieldConfig.defaults.custom.stacking.withMode('normal')
      + panel.timeSeries.fieldConfig.defaults.custom.withFillOpacity(10)
      + panel.timeSeries.fieldConfig.defaults.custom.withShowPoints('never')
      + panel.timeSeries.options.legend.withShowLegend(false)
      + panel.timeSeries.options.tooltip.withMode('multi')
      + panel.timeSeries.queryOptions.withDatasource('prometheus', '$datasource'),

    'alertmanager-overview.json':
      local alerts =
        panel.timeSeries.new('Alerts')
        + panel.timeSeries.panelOptions.withDescription('current set of alerts stored in the Alertmanager')
        + panel.timeSeries.standardOptions.withUnit('none')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'sum(alertmanager_alerts{%(amQuerySelector)s}) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config { amQuerySelector: amQuerySelector },
            )
            + prometheus.withIntervalFactor(2)
            + prometheus.withLegendFormat('%(amNameDashboardLegend)s' % $._config { amNameDashboardLegend: amNameDashboardLegend }),
          ]);

      local alertsRate =
        panel.timeSeries.new('Alerts receive rate')
        + panel.timeSeries.panelOptions.withDescription('rate of successful and invalid alerts received by the Alertmanager')
        + panel.timeSeries.standardOptions.withUnit('ops')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'sum(rate(alertmanager_alerts_received_total{%(amQuerySelector)s}[$__rate_interval])) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config { amQuerySelector: amQuerySelector },
            )
            + prometheus.withIntervalFactor(2)
            + prometheus.withLegendFormat('%(amNameDashboardLegend)s Received' % $._config { amNameDashboardLegend: amNameDashboardLegend }),
            prometheus.new(
              '$datasource',
              'sum(rate(alertmanager_alerts_invalid_total{%(amQuerySelector)s}[$__rate_interval])) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config { amQuerySelector: amQuerySelector },
            )
            + prometheus.withIntervalFactor(2)
            + prometheus.withLegendFormat('%(amNameDashboardLegend)s Invalid' % $._config { amNameDashboardLegend: amNameDashboardLegend }),
          ]);

      local notifications =
        panel.timeSeries.new('$integration: Notifications Send Rate')
        + panel.timeSeries.panelOptions.withDescription('rate of successful and invalid notifications sent by the Alertmanager')
        + panel.timeSeries.standardOptions.withUnit('ops')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.panelOptions.withRepeat('integration')
        + panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'sum(rate(alertmanager_notifications_total{%(amQuerySelector)s, integration="$integration"}[$__rate_interval])) by (integration,%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config { amQuerySelector: amQuerySelector },
            )
            + prometheus.withIntervalFactor(2)
            + prometheus.withLegendFormat('%(amNameDashboardLegend)s Total' % $._config { amNameDashboardLegend: amNameDashboardLegend }),
            prometheus.new(
              '$datasource',
              'sum(rate(alertmanager_notifications_failed_total{%(amQuerySelector)s, integration="$integration"}[$__rate_interval])) by (integration,%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config { amQuerySelector: amQuerySelector },
            )
            + prometheus.withIntervalFactor(2)
            + prometheus.withLegendFormat('%(amNameDashboardLegend)s Failed' % $._config { amNameDashboardLegend: amNameDashboardLegend }),
          ]);

      local notificationDuration =
        panel.timeSeries.new('$integration: Notification Duration')
        + panel.timeSeries.panelOptions.withDescription('latency of notifications sent by the Alertmanager')
        + panel.timeSeries.standardOptions.withUnit('s')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.panelOptions.withRepeat('integration')
        + panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              |||
                histogram_quantile(0.99,
                  sum(rate(alertmanager_notification_latency_seconds_bucket{%(amQuerySelector)s, integration="$integration"}[$__rate_interval])) by (le,%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)
                )
              ||| % $._config { amQuerySelector: amQuerySelector },
            )
            + prometheus.withIntervalFactor(2)
            + prometheus.withLegendFormat('%(amNameDashboardLegend)s 99th Percentile' % $._config { amNameDashboardLegend: amNameDashboardLegend }),
            prometheus.new(
              '$datasource',
            |||
              histogram_quantile(0.50,
                sum(rate(alertmanager_notification_latency_seconds_bucket{%(amQuerySelector)s, integration="$integration"}[$__rate_interval])) by (le,%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)
              )
            ||| % $._config { amQuerySelector: amQuerySelector },
            )
            + prometheus.withIntervalFactor(2)
            + prometheus.withLegendFormat('%(amNameDashboardLegend)s Median' % $._config { amNameDashboardLegend: amNameDashboardLegend }),
            prometheus.new(
              '$datasource',
              |||
                sum(rate(alertmanager_notification_latency_seconds_sum{%(amQuerySelector)s, integration="$integration"}[$__rate_interval])) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)
                /
                sum(rate(alertmanager_notification_latency_seconds_count{%(amQuerySelector)s, integration="$integration"}[$__rate_interval])) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)
              ||| % $._config { amQuerySelector: amQuerySelector },
            )
            + prometheus.withIntervalFactor(2)
            + prometheus.withLegendFormat('%(amNameDashboardLegend)s Average' % $._config { amNameDashboardLegend: amNameDashboardLegend }),
          ]);

      dashboard.new('%sOverview' % $._config.dashboardNamePrefix)
      + dashboard.time.withFrom('now-1h')
      + dashboard.withTags($._config.dashboardTags)
      + dashboard.withTimezone('utc')
      + dashboard.timepicker.withRefreshIntervals('30s')
      + dashboard.graphTooltip.withSharedCrosshair()
      + dashboard.withUid('alertmanager-overview')
      + dashboard.withVariables(
        [datasource]
        + alertmanagerClusterSelectorVariables
        + [integrationVariable]
        )
      + dashboard.withPanels(
          grafana.util.grid.makeGrid([
            row.new('Alerts')
            + row.withPanels([
              alerts,
              alertsRate
              ]),
            row.new('Notifications')
            + row.withPanels([
              notifications,
              notificationDuration
              ])
          ], panelWidth=12,  panelHeight=7)
        )
  },
}
