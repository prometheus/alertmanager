---
relations:
  - entity: alert-group
    verb: is notified for
    cardinality: one-to-many
references:
  - kind: code
    role: implementation
    target: config/config.go#Receiver
  - kind: doc
    role: intent
    target: docs/integrations.md
---

# Receiver

A named destination for notifications, holding one or more integrations:
email, Discord, incident.io, Jira, Mattermost, Microsoft Teams, Opsgenie,
PagerDuty, Pushover, Rocket.Chat, Slack, Amazon SNS, Telegram, VictorOps,
Webex, WeChat or a webhook.

## Information kept

- **Name** — the unique name routes refer to
- **Labels** — labels describing the receiver, such as its owning team
- **Integrations** — the notification integrations and their settings, each with its own templates
- **Send resolved** — per integration, whether it is also told when alerts resolve
