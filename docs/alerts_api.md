---
title: Alerts API
sort_rank: 6
nav_icon: sliders
---

**Important**: Prometheus takes care of sending alerts to the Alertmanager.
It is recommended to configure alerting rules in Prometheus based on time
series data instead of sending alerts to the Alerts API, as Prometheus supports
a number of special cases to make sure alerts are delivered even if Alertmanager
crashes or restarts.

You send alerts to Alertmanager via APIv2. The APIv2 is specified as an
OpenAPI specification that can be found [here](https://github.com/prometheus/alertmanager/blob/master/api/v2/openapi.yaml).

APIv1 was deprecated in Alertmanager version 0.16.0 and removed in Alertmanager
version 0.27.0.

To send alerts to APIv2 make a POST request to `api/v2/alerts`. You must set
the `Content-Type` header to `application/json`, and send JSON data containing
an array of alerts.

Here is an example:

```json
[
  {
    "labels": {
      "alertname": "<required_value>",
      "<name>": "<value>",
      ...
    },
    "annotations": {
      "<name>": "<value>",
    },
    "startsAt": "<RFC3339>",
    "endsAt": "<RFC3339>",
    "generatorURL": "<value>"
  },
  ...
]
```

All alerts have labels, annotations, an optional `startsAt` timestamp and an
optional `endsAt` timestamp. All timestamps are expected in the RFC3339 format.

Labels are used to deduplicate identical instances of the same alert, while
annotations are used to include other information about the alert, such as a
summary, description or a URL to a runbook.

The `startsAt` timestamp is the time the alert fired. If omitted, Alertmanager
sets `startsAt` to the current time.

The `endsAt` timestamp is the time the alert should be resolved. If omitted,
Alertmanager sets `endsAt` to the current time + `resolve_timeout`.

The `generatorURL` is a unique URL which links to the source of the alert. For
example, it might link to the firing rule in Prometheus.

## Listing alerts

To list current alerts, make a GET request to `/api/v2/alerts`. The query
parameters are defined in the [OpenAPI specification](https://github.com/prometheus/alertmanager/blob/main/api/v2/openapi.yaml).
An interactive HTML version is available in the [Swagger UI](http://petstore.swagger.io/?url=https://raw.githubusercontent.com/prometheus/alertmanager/main/api/v2/openapi.yaml).

Without query parameters this returns the full list of alerts. The boolean
parameters `active`, `silenced`, `inhibited`, and `unprocessed` all default to
`true`:

```
curl http://localhost:9093/api/v2/alerts
```

Filter alerts with the `filter` query parameter. Each value is a matcher
expression, for example `alertname="MyAlert"`. Repeat the parameter to apply
multiple matchers:

```
curl 'http://localhost:9093/api/v2/alerts?filter=alertname="MyAlert"'
curl 'http://localhost:9093/api/v2/alerts?filter=alertname="MyAlert"&filter=severity="critical"'
```

Query parameters such as `name=` or `alertname=` are not filters and are
ignored. Use `filter` as shown above.

The `receiver` query parameter is a regular expression matching receiver names:

```
curl 'http://localhost:9093/api/v2/alerts?receiver=team-.*'
```

Setting `active=false` excludes active alerts and returns only suppressed
(silenced or inhibited) alerts:

```
curl 'http://localhost:9093/api/v2/alerts?active=false'
```

## Expectations from clients

Clients are expected to re-send firing alerts to the Alertmanager at regular
intervals until the alert is resolved.

The exact interval depends on a number of variables such as the `endsAt`
timestamp, or if omitted the value of `resolve_timeout`. If the `endsAt`
timestamp is omitted, the Alertmanager will update the existing `endsAt`
timestamp for the alert to the current time + `resolve_timeout`.

Firing alerts are resolved once their `endsAt` timestamp has elapsed.

To ensure resolved notifications are sent for resolved alerts, clients are also
expected to re-send resolved alerts to the Alertmanager for up to 5 minutes
after the alert has resolved. As the Alertmanager is stateless, this ensures
that a resolved notification is sent even if the Alertmanager crashes or is
restarted.
