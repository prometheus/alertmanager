---
id: alertmanager
summary: Receives alerts from monitoring systems, groups and routes them to the right receivers, and quiets the noise with silences, inhibition and time intervals.
category: alerting
tags: [alerting, monitoring, notifications, prometheus]
authors:
  - name: The Prometheus Authors
    url: https://prometheus.io
license: Apache-2.0
languages: [en]
limitations:
  - Alertmanager keeps no accounts or roles. Whoever its web configuration admits — every caller when it sets no basic authentication users and no client certificates — can do everything its web UI and API offer.
  - The creator named on a silence is text the person types; it grants no ownership.
  - Routes, receivers, inhibition rules, time intervals and notification templates are written by operators in the configuration file; nothing in Alertmanager edits them.
  - Alertmanager does not evaluate alerting rules; alert generators such as Prometheus decide when an alert fires and resolves.
  - Alerts are kept in memory only; alert generators keep resending firing alerts, so a restarted Alertmanager learns them again.
  - Each Alertmanager in a cluster receives alerts on its own and loads its own configuration file; peers share only silences and the notification log.
  - Notifications are sent at least once; while peers cannot reach each other, each side notifies on its own and receivers may get duplicates.
  - Notifications are delivered by external services such as email, chat, paging and webhooks; Alertmanager only sends to them.
  - API v1 was removed; its paths answer only with a deprecation notice.
references:
  - kind: doc
    role: intent
    target: docs/alertmanager.md
    title: Alertmanager concepts
  - kind: doc
    role: intent
    target: docs/high_availability.md
  - kind: doc
    role: context
    target: docs/https.md
  - kind: doc
    role: context
    target: README.md
  - kind: doc
    role: context
    target: AGENTS.md
---

# Alertmanager

Alertmanager handles alerts sent by alert generators such as the Prometheus
server. It deduplicates them, groups alerts of a similar nature, routes each
group to the receivers the configuration names, and sends notifications
through integrations such as email, PagerDuty, Opsgenie, Slack or a webhook.
Operators silence alerts for a time, inhibition rules mute alerts while a
related alert is already firing, and time intervals limit when a route
notifies. Several Alertmanagers can run as one highly available cluster.

## Intent

During a large outage hundreds of alerts may fire at once. Alertmanager turns
them into a few compact notifications sent to the right people, and lets the
people on call quiet what they already know about.
