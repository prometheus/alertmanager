---
title: Notification Integrations
sort_rank: 4
---

Alertmanager supports a number of notification integrations via the [configuration file](configuration.md).

## Available Integrations

| Name | Configuration | External Configuration | API Reference |
|------|---------------|-------------------------------------|---------------|
| [Amazon SNS](https://aws.amazon.com/sns/) | [sns_config](configuration.md#sns_config) | [Amazon SNS Documentation](https://docs.aws.amazon.com/sns/) | [SNS API Reference](https://docs.aws.amazon.com/sns/latest/api/welcome.html) |
| [Discord](https://discord.com/) | [discord_config](configuration.md#discord_config) | [Intro to Webhooks](https://support.discord.com/hc/en-us/articles/228383668-Intro-to-Webhooks) | [Discord Webhook API](https://discord.com/developers/docs/resources/webhook) |
| [Email](https://en.wikipedia.org/wiki/Email) | [email_config](configuration.md#email_config) | - | [SMTP](https://en.wikipedia.org/wiki/Simple_Mail_Transfer_Protocol) |
| [incident.io](https://incident.io/) | [incidentio_config](configuration.md#incidentio_config) | [Alert Sources Documentation](https://api-docs.incident.io/tag/Alert-Sources-V2) | [Alert Sources V2 API](https://api-docs.incident.io/tag/Alert-Sources-V2#operation/Alert%20Sources%20V2_Create) |
| [Jira](https://www.atlassian.com/software/jira) | [jira_config](configuration.md#jira_config) | [Jira Cloud Platform](https://developer.atlassian.com/cloud/jira/platform/) | [REST API v2](https://developer.atlassian.com/cloud/jira/platform/rest/v2/intro/) / [REST API v3](https://developer.atlassian.com/cloud/jira/platform/rest/v3/intro/) |
| [Mattermost](https://mattermost.com/) | [mattermost_config](configuration.md#mattermost_config) | [Incoming Webhooks](https://developers.mattermost.com/integrate/webhooks/incoming/) | [Mattermost Webhook API](https://developers.mattermost.com/integrate/webhooks/incoming/) |
| [Microsoft Teams](https://www.microsoft.com/en-us/microsoft-teams/) | [msteams_config](configuration.md#msteams_config) | [Incoming Webhooks (Deprecated)](https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/what-are-webhooks-and-connectors) | [Microsoft Teams Connectors](https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/what-are-webhooks-and-connectors) |
| [Microsoft Teams v2](https://www.microsoft.com/en-us/microsoft-teams/) | [msteamsv2_config](configuration.md#msteamsv2_config) | [Workflows for Teams](https://support.microsoft.com/en-gb/office/create-incoming-webhooks-with-workflows-for-microsoft-teams-8ae491c7-0394-4861-ba59-055e33f75498) | [Power Automate Flows](https://learn.microsoft.com/en-us/power-automate/teams/overview) |
| [OpsGenie](https://www.atlassian.com/software/opsgenie) | [opsgenie_config](configuration.md#opsgenie_config) | [OpsGenie Documentation](https://docs.opsgenie.com/) | [OpsGenie Alert API](https://docs.opsgenie.com/docs/alert-api) |
| [PagerDuty](https://www.pagerduty.com/) | [pagerduty_config](configuration.md#pagerduty_config) | [Prometheus Integration Guide](https://www.pagerduty.com/docs/guides/prometheus-integration-guide/) | [PagerDuty Events API](https://developer.pagerduty.com/documentation/integration/events) |
| [Pushover](https://pushover.net/) | [pushover_config](configuration.md#pushover_config) | [Pushover Documentation](https://pushover.net/api) | [Pushover API](https://pushover.net/api) |
| [Rocket.Chat](https://rocket.chat/) | [rocketchat_config](configuration.md#rocketchat_config) | [Personal Access Tokens](https://docs.rocket.chat/use-rocket.chat/user-guides/user-panel/my-account#personal-access-tokens) | [Rocket.Chat REST API](https://developer.rocket.chat/reference/api/rest-api/endpoints/messaging/chat-endpoints/postmessage) |
| [Slack](https://slack.com/) | [slack_config](configuration.md#slack_config) | [Incoming Webhooks](https://api.slack.com/messaging/webhooks) / [Bot Tokens](https://api.slack.com/authentication/token-types) | [Slack API](https://api.slack.com/methods/chat.postMessage) |
| [Telegram](https://telegram.org/) | [telegram_config](configuration.md#telegram_config) | [Telegram Bots](https://core.telegram.org/bots) | [Telegram Bot API](https://core.telegram.org/bots/api) |
| [VictorOps](https://victorops.com/) | [victorops_config](configuration.md#victorops_config) | [REST Endpoint Integration Guide](https://help.victorops.com/knowledge-base/rest-endpoint-integration-guide/) | [VictorOps REST API](https://help.victorops.com/knowledge-base/rest-endpoint-integration-guide/) |
| [Webex](https://www.webex.com/) | [webex_config](configuration.md#webex_config) | [Webex for Developers](https://developer.webex.com/) | [Webex Messages API](https://developer.webex.com/docs/api/v1/messages) |
| [Webhook](https://en.wikipedia.org/wiki/Webhook) | [webhook_config](configuration.md#webhook_config) | [Webhook Integrations](https://prometheus.io/docs/operating/integrations/#alertmanager-webhook-receiver) | - |
| [WeChat](https://www.wechat.com/) | [wechat_config](configuration.md#wechat_config) | [WeChat Work Documentation](https://developers.weixin.qq.com/doc/offiaccount/en/Message_Management/Service_Center_messages.html) | [WeChat Work API](https://developers.weixin.qq.com/doc/offiaccount/en/Message_Management/Service_Center_messages.html) |

