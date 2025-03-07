## 0.28.1 / 2025-03-07

* [ENHANCEMENT] Improved performance of inhibition rules when using Equal labels. #4119
* [ENHANCEMENT] Improve the documentation on escaping in UTF-8 matchers. #4157
* [ENHANCEMENT] Update alertmanager_config_hash metric help to document the hash is not cryptographically strong. #4210
* [BUGFIX] Fix panic in amtool when using `--verbose`. #4218
* [BUGFIX] Fix templating of channel field for Rocket.Chat. #4220
* [BUGFIX] Fix `rocketchat_configs` written as `rocket_configs` in docs. #4217
* [BUGFIX] Fix usage for `--enable-feature` flag. #4214
* [BUGFIX] Trim whitespace from OpsGenie API Key. #4195
* [BUGFIX] Fix Jira project template not rendered when searching for existing issues. #4291
* [BUGFIX] Fix subtle bug in JSON/YAML encoding of inhibition rules that would cause Equal labels to be omitted. #4292
* [BUGFIX] Fix header for `slack_configs` in docs. #4247
* [BUGFIX] Fix weight and wrap of Microsoft Teams notifications. #4222
* [BUGFIX] Fix format of YAML examples in configuration.md. #4207

## 0.28.0 / 2025-01-15

* [CHANGE] Templating errors in the SNS integration now return an error. #3531 #3879
* [CHANGE] Adopt log/slog, drop go-kit/log #4089
* [FEATURE] Add a new Microsoft Teams integration based on Flows #4024
* [FEATURE] Add a new Rocket.Chat integration #3600
* [FEATURE] Add a new Jira integration #3590 #3931
* [FEATURE] Add support for `GOMEMLIMIT`, enable it via the feature flag `--enable-feature=auto-gomemlimit`. #3895
* [FEATURE] Add support for `GOMAXPROCS`, enable it via the feature flag `--enable-feature=auto-gomaxprocs`. #3837
* [FEATURE] Add support for limits of silences including the maximum number of active and pending silences, and the maximum size per silence (in bytes). You can use the flags `--silences.max-silences` and `--silences.max-silence-size-bytes` to set them accordingly #3852 #3862 #3866 #3885 #3886 #3877
* [FEATURE] Muted alerts now show whether they are suppressed or not in both the `/api/v2/alerts` endpoint and the Alertmanager UI. #3793 #3797 #3792
* [ENHANCEMENT] Add support for `content`, `username` and `avatar_url` in the Discord integration. `content` and `username` also support templating. #4007
* [ENHANCEMENT] Only invalidate the silences cache if a new silence is created or an existing silence replaced - should improve latency on both `GET api/v2/alerts` and `POST api/v2/alerts` API endpoint. #3961
* [ENHANCEMENT] Add image source label to Dockerfile. To get changelogs shown when using Renovate #4062
* [ENHANCEMENT] Build using go 1.23 #4071
* [ENHANCEMENT] Support setting a global SMTP TLS configuration. #3732
* [ENHANCEMENT] The setting `room_id` in the WebEx integration can now be templated to allow for dynamic room IDs. #3801
* [ENHANCEMENT] Enable setting `message_thread_id` for the Telegram integration. #3638
* [ENHANCEMENT] Support the `since` and `humanizeDuration` functions to templates. This means users can now format time to more human-readable text. #3863
* [ENHANCEMENT] Support the `date` and `tz` functions to templates. This means users can now format time in a specified format and also change the timezone to their specific locale. #3812
* [ENHANCEMENT] Latency metrics now support native histograms. #3737
* [ENHANCEMENT] Add full width to adaptive card for msteamsv2 #4135
* [ENHANCEMENT] Add timeout option for webhook notifier. #4137
* [ENHANCEMENT] Update config to allow showing secret values when marshaled #4158
* [ENHANCEMENT] Enable templating for Jira project and issue_type #4159
* [BUGFIX] Fix the SMTP integration not correctly closing an SMTP submission, which may lead to unsuccessful dispatches being marked as successful. #4006
* [BUGFIX]  The `ParseMode` option is now set explicitly in the Telegram integration. If we don't HTML tags had not been parsed by default. #4027
* [BUGFIX] Fix a memory leak that was caused by updates silences continuously. #3930
* [BUGFIX] Fix hiding secret URLs when the URL is incorrect. #3887
* [BUGFIX] Fix a race condition in the alerts - it was more of a hypothetical race condition that could have occurred in the alert reception pipeline. #3648
* [BUGFIX] Fix a race condition in the alert delivery pipeline that would cause a firing alert that was delivered earlier to be deleted from the aggregation group when instead it should have been delivered again. #3826
* [BUGFIX] Fix version in APIv1 deprecation notice. #3815
* [BUGFIX] Fix crash errors when using `url_file` in the Webhook integration. #3800
* [BUGFIX] fix `Route.ID()` returns conflicting IDs. #3803
* [BUGFIX] Fix deadlock on the alerts memory store. #3715
* [BUGFIX] Fix `amtool template render` when using the default values. #3725
* [BUGFIX] Fix `webhook_url_file` for both the Discord and Microsoft Teams integrations. #3728 #3745
* [BUGFIX] Fix wechat api link #4084
* [BUGFIX] Fix build info metric #4166
* [BUGFIX] Fix UTF-8 not allowed in Equal field for inhibition rules #4177

## 0.27.0 / 2024-02-28

* [CHANGE] Discord Integration: Enforce max length in `message`. #3597
* [CHANGE] API: Removal of all `api/v1/` endpoints. These endpoints now log and return a deprecation message and respond with a status code of `410`. #2970
* [FEATURE] UTF-8 Support: Introduction of support for any UTF-8 character as part of label names and matchers. Please read more below. #3453, #3483, #3567, #3570
* [FEATURE] Metrics: Introduced the experimental feature flag `--enable-feature=receiver-name-in-metrics` to include the receiver name in the following metrics: #3045
  * `alertmanager_notifications_total`
  * `alertmanager_notifications_failed_totall`
  * `alertmanager_notification_requests_total`
  * `alertmanager_notification_requests_failed_total`
  * `alertmanager_notification_latency_seconds`
* [FEATURE] Metrics: Introduced a new gauge named `alertmanager_inhibition_rules` that counts the number of configured inhibition rules. #3681
* [FEATURE] Metrics: Introduced a new counter named `alertmanager_alerts_supressed_total` that tracks muted alerts, it contains a `reason` label to indicate the source of the mute. #3565
* [ENHANCEMENT] Discord Integration: Introduced support for `webhook_url_file`. #3555
* [ENHANCEMENT] Microsoft Teams Integration: Introduced support for `webhook_url_file`. #3555
* [ENHANCEMENT] Microsoft Teams Integration: Add support for `summary`. #3616
* [ENHANCEMENT] Metrics: Notification metrics now support two new values for the label `reason`, `contextCanceled` and `contextDeadlineExceeded`. #3631
* [ENHANCEMENT] Email Integration: Contents of `auth_password_file` are now trimmed of prefixed and suffixed whitespace. #3680
* [BUGFIX] amtool: Fixes the error `scheme required for webhook url` when using amtool with `--alertmanager.url`. #3509
* [BUGFIX] Mixin: Fix `AlertmanagerFailedToSendAlerts`, `AlertmanagerClusterFailedToSendAlerts`, and `AlertmanagerClusterFailedToSendAlerts` to make sure they ignore the `reason` label. #3599

### Removal of API v1

The Alertmanager `v1` API has been deprecated since January 2019 with the release of Alertmanager `v0.16.0`. With the release of version `0.27.0` it is now removed.
A successful HTTP request to any of the `v1` endpoints will log and return a deprecation message while responding with a status code of `410`.
Please ensure you switch to the `v2` equivalent endpoint in your integrations before upgrading.

### Alertmanager support for all UTF-8 characters in matchers and label names

Starting with Alertmanager `v0.27.0`, we have a new parser for matchers that has a number of backwards incompatible changes. While most matchers will be forward-compatible, some will not. Alertmanager is operating a transition period where it supports both UTF-8 and classic matchers, so **it's entirely safe to upgrade without any additional configuration**. With that said, we recommend the following:

- If this is a new Alertmanager installation, we recommend enabling UTF-8 strict mode before creating an Alertmanager configuration file. You can enable strict mode with `alertmanager --config.file=config.yml --enable-feature="utf8-strict-mode"`.

- If this is an existing Alertmanager installation, we recommend running the Alertmanager in the default mode called fallback mode before enabling UTF-8 strict mode. In this mode, Alertmanager will log a warning if you need to make any changes to your configuration file before UTF-8 strict mode can be enabled. **Alertmanager will make UTF-8 strict mode the default in the next two versions**, so it's important to transition as soon as possible.

Irrespective of whether an Alertmanager installation is a new or existing installation, you can also use `amtool` to validate that an Alertmanager configuration file is compatible with UTF-8 strict mode before enabling it in Alertmanager server by running `amtool check-config config.yml` and inspecting the log messages.

Should you encounter any problems, you can run the Alertmanager with just the classic parser enabled by running `alertmanager --config.file=config.yml --enable-feature="classic-mode"`. If so, please submit a bug report via GitHub issues.

## 0.26.0 / 2023-08-23

* [SECURITY] Fix stored XSS via the /api/v1/alerts endpoint in the Alertmanager UI. CVE-2023-40577
* [CHANGE] Telegram Integration: `api_url` is now optional. #2981
* [CHANGE] Telegram Integration: `ParseMode` default is now `HTML` instead of `MarkdownV2`. #2981
* [CHANGE] Webhook Integration: `url` is now marked as a secret. It will no longer show up in the logs as clear-text. #3228
* [CHANGE] Metrics: New label `reason` for `alertmanager_notifications_failed_total` metric to indicate the type of error of the alert delivery. #3094 #3307
* [FEATURE] Clustering: New flag `--cluster.label`, to help to block any traffic that is not meant for the cluster. #3354
* [FEATURE] Integrations: Add Microsoft Teams as a supported integration. #3324
* [ENHANCEMENT] Telegram Integration: Support `bot_token_file` for loading this secret from a file. #3226
* [ENHANCEMENT] Webhook Integration: Support `url_file` for loading this secret from a file. #3223
* [ENHANCEMENT] Webhook Integration: Leading and trailing white space is now removed for the contents of `url_file`. #3363
* [ENHANCEMENT] Pushover Integration: Support options `device` and `sound` (sound was previously supported but undocumented). #3318
* [ENHANCEMENT] Pushover Integration: Support `user_key_file` and `token_file` for loading this secret from a file. #3200
* [ENHANCEMENT] Slack Integration: Support errors wrapped in successful (HTTP status code 200) responses. #3121
* [ENHANCEMENT] API: Add `CORS` and `Cache-Control` HTTP headers to all version 2 API routes. #3195
* [ENHANCEMENT] UI: Receiver name is now visible as part of the alerts page. #3289
* [ENHANCEMENT] Templating: Better default text when using `{{ .Annotations }}` and `{{ .Labels }}`. #3256
* [ENHANCEMENT] Templating: Introduced a new function `trimSpace` which removes leading and trailing white spaces. #3223
* [ENHANCEMENT] CLI: `amtool silence query` now supports the `--id` flag to query an individual silence. #3241
* [ENHANCEMENT] Metrics: Introduced `alertmanager_nflog_maintenance_total` and `alertmanager_nflog_maintenance_errors_total` to monitor maintenance of the notification log. #3286 
* [ENHANCEMENT] Metrics: Introduced `alertmanager_silences_maintenance_total` and `alertmanager_silences_maintenance_errors_total` to monitor maintenance of silences. #3285
* [ENHANCEMENT] Logging: Log GroupKey and alerts on alert delivery when using debug mode. #3438
* [BUGFIX] Configuration: Empty list of `receivers` and `inhibit_rules` would cause the alertmanager to crash. #3209 
* [BUGFIX] Templating: Fixed a race condition when using the `title` function. It is now race-safe. #3278
* [BUGFIX] API: Fixed duplicate receiver names in the `api/v2/receivers` API endpoint. #3338
* [BUGFIX] API: Attempting to delete a silence now returns the correct status code, `404` instead of `500`. #3352
* [BUGFIX] Clustering: Fixes a panic when `tls_client_config` is empty. #3443
* [BUGFIX] Fix stored XSS via the /api/v1/alerts endpoint in the Alertmanager UI.

## 0.25.0 / 2022-12-22

* [CHANGE] Change the default `parse_mode` value from `MarkdownV2` to `HTML` for Telegram. #2981
* [CHANGE] Make `api_url` field optional for Telegram. #2981
* [CHANGE] Use CanonicalMIMEHeaderKey instead of TitleCasing for email headers. #3080
* [CHANGE] Reduce the number of notification logs broadcasted between peers by expiring them after (2 * repeat interval). #2982
* [FEATURE] Add `proxy_url` support for OAuth2 in HTTP client configuration. #3010
* [FEATURE] Reload TLS certificate and key from disk when updated. #3168
* [FEATURE] Add Discord integration. #2948
* [FEATURE] Add Webex integration. #3132
* [ENHANCEMENT] Add `--web.systemd-socket` flag to systemd socket activation listeners instead of port listeners (Linux only). #3140
* [ENHANCEMENT] Add `enable_http2` support in HTTP client configuration. #3010
* [ENHANCEMENT] Add `min_version` support to select the minimum TLS version in HTTP client configuration. #3010
* [ENHANCEMENT] Add `max_version` support to select the maximum TLS version in HTTP client configuration. #3168
* [ENHANCEMENT] Emit warning logs when truncating messages in notifications. #3145
* [ENHANCEMENT] Add `--data.maintenance-interval` flag to define the interval between the garbage collection and snapshotting to disk of the silences and the notification logs. #2849
* [ENHANCEMENT] Support HEAD method for the `/-/healty` and `/-/ready` endpoints. #3039
* [ENHANCEMENT] Truncate messages with the `…` ellipsis character instead of the 3-dots string `...`. #3072
* [ENHANCEMENT] Add support for reading global and local SMTP passwords from files. #3038
* [ENHANCEMENT] Add Location support to time intervals. #2782
* [ENHANCEMENT] UI: Add 'Link' button to alerts in list. #2880
* [ENHANCEMENT] Add the `source` field to the PagerDuty configuration. #3106
* [ENHANCEMENT] Add support for reading PagerDuty routing and service keys from files. #3107
* [ENHANCEMENT] Log response details when notifications fail for Webhooks, Pushover and VictorOps. #3103
* [ENHANCEMENT] UI: Allow to choose the first day of the week as Sunday or Monday. #3093
* [ENHANCEMENT] Add support for reading VictorOps API key from file. #3111
* [ENHANCEMENT] Support templating for Opsgenie's responder type. #3060
* [BUGFIX] Fail configuration loading if `api_key` and `api_key_file` are defined at the same time. #2910
* [BUGFIX] Fix the `alertmanager_alerts` metric to avoid counting resolved alerts as active. Also added a new `alertmanager_marked_alerts` metric that retain the old behavior. #2943
* [BUGFIX] Trim contents of Slack API URLs when reading from files. #2929
* [BUGFIX] amtool: Avoid panic when the label value matcher is empty. #2968
* [BUGFIX] Fail configuration loading if `api_url` is empty for OpsGenie. #2910
* [BUGFIX] Fix email template for resolved notifications. #3166
* [BUGFIX] Use the HTML template engine when the parse mode is HTML for Telegram. #3183

## 0.24.0 / 2022-03-24

* [CHANGE] Add the `/api/v2` prefix to all endpoints in the OpenAPI specification and generated client code. #2696
* [CHANGE] Remove the `github.com/prometheus/alertmanager/client` Go package. #2763
* [FEATURE] Add `--cluster.tls-config` experimental flag to secure cluster traffic via mutual TLS. #2237
* [FEATURE] Add support for active time intervals. Active and mute time intervals should be defined via `time_intervals` rather than `mute_time_intervals` (the latter is deprecated but it will be supported until v1.0). #2779
* [FEATURE] Add Telegram integration. #2827
* [ENHANCEMENT] Add `update_alerts` field to the OpsGenie configuration to update message and description when sending alerts. #2519
* [ENHANCEMENT] Add `--cluster.allow-insecure-public-advertise-address-discovery` feature flag to enable discovery and use of public IP addresses for clustering. #2719
* [ENHANCEMENT] Add `entity` and `actions` fields to the OpsGenie configuration. #2753
* [ENHANCEMENT] Add `opsgenie_api_key_file` field to the global configuration. #2728
* [ENHANCEMENT] Add support for `teams` responders to the OpsGenie configuration. #2685
* [ENHANCEMENT] Add the User-Agent header to all notification requests. #2730
* [ENHANCEMENT] Re-enable HTTP/2. #2720
* [ENHANCEMENT] web: Add support for security-related HTTP headers. #2759
* [ENHANCEMENT] amtool: Allow filtering of silences by `createdBy` author. #2718
* [ENHANCEMENT] amtool: add `--http.config.file` flag to configure HTTP settings. #2764
* [BUGFIX] Fix HTTP client configuration for the SNS receiver. #2706
* [BUGFIX] Fix unclosed file descriptor after reading the silences snapshot file. #2710
* [BUGFIX] Fix field names for `mute_time_intervals` in JSON marshaling. #2765
* [BUGFIX] Ensure that the root route doesn't have any matchers. #2780
* [BUGFIX] Truncate the message's title to 1024 chars to avoid hitting Slack limits. #2774
* [BUGFIX] Fix the default HTML email template (`email.default.html`) to match with the canonical source. #2798
* [BUGFIX] Detect SNS FIFO topic based on the rendered value. #2819
* [BUGFIX] Avoid deleting and recreating a silence when an update is possible. #2816
* [BUGFIX] api/v2: Return 200 OK when deleting an expired silence. #2817
* [BUGFIX] amtool: Fix the silence's end date when adding a silence. The end date is (start date + duration) while it used to be (current time + duration). The new behavior is consistent with the update operation. #2741

## 0.23.0 / 2021-08-25

* [FEATURE] Add AWS SNS receiver. #2615
* [FEATURE] amtool: add new template render command. #2538
* [ENHANCEMENT] amtool: Add ability to skip TLS verification for amtool. #2663
* [ENHANCEMENT] amtool: Detect version drift and warn users. #2672
* [BUGFIX] Time-based muting: Ensure time interval comparisons are in UTC. #2648
* [BUGFIX] amtool: Fix empty isEqual when talking to incompatible alertmanager. #2668

## 0.22.2 / 2021-06-01

* [BUGFIX] Include pending silences for future muting decisions. #2590

## 0.22.1 / 2021-05-27

This release addresses a regression in the API v1 that was introduced in 0.22.0.
Matchers in silences created with the API v1 could be considered negative
matchers. This affects users using amtool prior to v0.17.0.

* [BUGFIX] API v1: Decode matchers without isEqual are positive matchers. #2603

## 0.22.0 / 2021-05-21

* [CHANGE] Amtool and Alertmanager binaries help now prints to stdout. #2505
* [CHANGE] Use path relative to the configuration file for certificates and password files. #2502
* [CHANGE] Display Silence and Alert dates in ISO8601 format. #2363
* [FEATURE] Add date picker to silence form views. #2262
* [FEATURE] Add support for negative matchers. #2434 #2460 and many more.
* [FEATURE] Add time-based muting to routing tree. #2393
* [FEATURE] Support TLS and basic authentication on the web server. #2446
* [FEATURE] Add OAuth 2.0 client support in HTTP client. #2560
* [ENHANCEMENT] Add composite durations in the configuration (e.g. 2h20m). #2353
* [ENHANCEMENT] Add follow_redirect option to disable following redirects. #2551
* [ENHANCEMENT] Add metric for permanently failed notifications. #2383
* [ENHANCEMENT] Add support for custom authorization scheme. #2499
* [ENHANCEMENT] Add support for not following HTTP redirects. #2499
* [ENHANCEMENT] Add support to set the Slack URL from a file. #2534
* [ENHANCEMENT] amtool: Add alert status to extended and simple output. #2324
* [ENHANCEMENT] Do not omit false booleans in the configuration page. #2317
* [ENHANCEMENT] OpsGenie: Propagate labels to Opsgenie details. #2276
* [ENHANCEMENT] PagerDuty: Filter out empty images and links. #2379
* [ENHANCEMENT] WeChat: add markdown support. #2309
* [BUGFIX] Fix a possible deadlock on shutdown. #2558
* [BUGFIX] UI: Fix extended printing of regex sign. #2445
* [BUGFIX] UI: Fix the favicon when using a path prefix. #2392
* [BUGFIX] Make filter labels consistent with Prometheus. #2403
* [BUGFIX] alertmanager_config_last_reload_successful takes templating failures into account. #2373
* [BUGFIX] amtool: avoid nil dereference in silence update. #2427
* [BUGFIX] VictorOps: Catch routing_key templating errors. #2467

## 0.21.0 / 2020-06-16

This release removes the HipChat integration as it is discontinued by Atlassian on June 30th 2020.

* [CHANGE] [HipChat] Remove HipChat integration as it is end-of-life. #2282
* [CHANGE] [amtool] Remove default assignment of environment variables. #2161
* [CHANGE] [PagerDuty] Enforce 512KB event size limit. #2225
* [ENHANCEMENT] [amtool] Add `cluster` command to show cluster and peer statuses. #2256
* [ENHANCEMENT] Add redirection from `/` to the routes prefix when it isn't empty. #2235
* [ENHANCEMENT] [Webhook] Add `max_alerts` option to limit the number of alerts included in the payload. #2274
* [ENHANCEMENT] Improve logs for API v2, notifications and clustering. #2177 #2188 #2260 #2261 #2273
* [BUGFIX] Fix child routes not inheriting their parent route's grouping when `group_by: [...]`. #2154
* [BUGFIX] [UI] Fix the receiver selector in the Alerts page when the receiver name contains regular expression metacharacters such as `+`. #2090
* [BUGFIX] Fix error message about start and end time validation. #2173
* [BUGFIX] Fix a potential race condition in dispatcher. #2208
* [BUGFIX] [API v2] Return an empty array of peers when the clustering is disabled. #2203
* [BUGFIX] Fix the registration of `alertmanager_dispatcher_aggregation_groups` and `alertmanager_dispatcher_alert_processing_duration_seconds` metrics. #2200
* [BUGFIX] Always retry notifications with back-off. #2290

## 0.20.0 / 2019-12-11

* [CHANGE] Check that at least one silence matcher matches a non-empty string. #2081
* [ENHANCEMENT] [pagerduty] Check that PagerDuty keys aren't empty. #2085
* [ENHANCEMENT] [template] Add the `stringSlice` function. #2101
* [ENHANCEMENT] Add `alertmanager_dispatcher_aggregation_groups` and `alertmanager_dispatcher_alert_processing_duration_seconds` metrics. #2113
* [ENHANCEMENT] Log unused receivers. #2114
* [ENHANCEMENT] Add `alertmanager_receivers` metric. #2114
* [ENHANCEMENT] Add `alertmanager_integrations` metric. #2117
* [ENHANCEMENT] [email] Add Message-Id Header to outgoing emails. #2057
* [BUGFIX] Don't garbage-collect alerts from the store. #2040
* [BUGFIX] [ui] Disable the grammarly plugin on all textareas. #2061
* [BUGFIX] [config] Forbid nil regexp matchers. #2083
* [BUGFIX] [ui] Fix Silences UI when several filters are applied. #2075

Contributors:

* @CharlesJUDITH
* @NotAFile
* @Pger-Y
* @TheMeier
* @johncming
* @n33pm
* @ntk148v
* @oddlittlebird
* @perlun
* @qoops-1
* @roidelapluie
* @simonpasquier
* @stephenreddek
* @sylr
* @vrischmann

## 0.19.0 / 2019-09-03

* [CHANGE] Reject invalid external URLs at startup. #1960
* [CHANGE] Add Fingerprint to template data. #1945
* [CHANGE] Check Smarthost validity at config loading. #1957
* [ENHANCEMENT] Improve error messages for email receiver. #1953
* [ENHANCEMENT] Log error messages from OpsGenie API. #1965
* [ENHANCEMENT] Add the ability to configure Slack markdown field. #1967
* [ENHANCEMENT] Log warning when repeat_interval > retention. #1993
* [ENHANCEMENT] Add `alertmanager_cluster_enabled` metric. #1973
* [ENHANCEMENT] [ui] Recreate silence with previous comment. #1927
* [BUGFIX] [ui] Fix /api/v2/alerts/groups endpoint with similar alert groups. #1964
* [BUGFIX] Allow slashes in receivers. #2011
* [BUGFIX] [ui] Fix expand/collapse button with identical alert groups. #2012

## 0.18.0 / 2019-07-08

* [CHANGE] Remove quantile labels from Summary metrics. #1921
* [CHANGE] [OpsGenie] Move from the deprecated `teams` field in the configuration to `responders`. #1863
* [CHANGE] [ui] Collapse alert groups on the initial view. #1876
* [CHANGE] [Wechat] Set the default API secret to blank. #1888
* [CHANGE/BUGFIX] [PagerDuty] Fix embedding of images, the `text` field in the configuration has been renamed to `href`. #1931
* [ENHANCEMENT] Use persistent HTTP clients. #1904
* [ENHANCEMENT] Add `alertmanager_cluster_alive_messages_total`, `alertmanager_cluster_peer_info` and `alertmanager_cluster_pings_seconds` metrics. #1941
* [ENHANCEMENT] [api] Add missing metrics for API v2. #1902
* [ENHANCEMENT] [Slack] Log error message on retry errors. #1655
* [ENHANCEMENT] [ui] Allow to create silences from the alerts filter bar. #1911
* [ENHANCEMENT] [ui] Enable auto resize the textarea fields. #1893
* [BUGFIX] [amtool] Use scheme, authentication and base path from the URL if present. #1892 #1940
* [BUGFIX] [amtool] Support filtering alerts by receiver. #1915
* [BUGFIX] [api] Fix /api/v2/alerts with multiple receivers. #1948
* [BUGFIX] [PagerDuty] Truncate description to 1024 chars for PagerDuty v1. #1922
* [BUGFIX] [ui] Add filtering based off of "active" query param. #1879


## 0.17.0 / 2019-05-02

This release includes changes to amtool which are not fully backwards
compatible with the previous amtool version (#1798) related to backup and
import of silences. If a backup of silences is created using a previous
version of amtool (v0.16.1 or earlier), it is possible that not all silences
can be correctly imported using a later version of amtool.

Additionally, the groups endpoint that was dropped from api v1 has been added
to api v2. The default for viewing alerts in the UI now consumes from this
endpoint and displays alerts grouped according to the groups defined in the
running configuration. Custom grouping is still supported.

This release has added two new flags that may need to be tweaked. For people
running with a lot of concurrent requests, consider increasing the value of
`--web.get-concurrency`. An increase in 503 errors indicates that the request
rate is exceeding the number of currently available workers. The other new
flag, --web.timeout, limits the time a request is allowed to run. The default
behavior is to not use a timeout.

* [CHANGE] Modify the self-inhibition prevention semantics (#1873)
* [CHANGE] Make api/v2/status.cluster.{name,peers} properties optional for Alertmanager with disabled clustering (#1728)
* [FEATURE] Add groups endpoint to v2 api (#1791)
* [FEATURE] Optional timeout for HTTP requests (#1743)
* [ENHANCEMENT] Set HTTP headers to prevent asset caching (#1817)
* [ENHANCEMENT] API returns current silenced/inhibited state of alerts (#1733)
* [ENHANCEMENT] Configurable concurrency limit for GET requests (#1743)
* [ENHANCEMENT] Pushover notifier: support HTML, URL title and custom sounds (#1634)
* [ENHANCEMENT] Support adding custom fields to VictorOps notifications (#1420)
* [ENHANCEMENT] Migrate amtool CLI to API v2 (#1798)
* [ENHANCEMENT][ui] Default alert list view grouped by configured alert groups (#1864)
* [ENHANCEMENT][ui] Remove superfluous inhibited/silenced text, show inhibited status (#1698, #1862)
* [ENHANCEMENT][ui] Silence preview now shows already-muted alerts (#1776)
* [ENHANCEMENT][ui] Sort silences from api/v2 similarly to api/v1 (#1786)
* [BUGFIX] Trim PagerDuty message summary to 1024 chars (#1701)
* [BUGFIX] Add fix for race causing alerts to be dropped (#1843)
* [BUGFIX][ui] Correctly construct filter query string for api (#1869)
* [BUGFIX][ui] Do not display GroupByAll and GroupBy in marshaled config (#1665)
* [BUGFIX][ui] Respect regex setting when creating silences (#1697)

## 0.16.2 / 2019-04-03

Updating to v0.16.2 is recommended for all users using the Slack, Pagerduty,
Hipchat, Wechat, VictorOps and Pushover notifier, as connection errors could
leak secrets embedded in the notifier's URL to stdout.

* [BUGFIX] Redact notifier URL from logs to not leak secrets embedded in the URL (#1822, #1825)
* [BUGFIX] Allow sending of unauthenticated SMTP requests when `smtp_auth_username` is not supplied (#1739)

## 0.16.1 / 2019-01-31

* [BUGFIX] Do not populate cluster info if clustering is disabled in API v2 (#1726)

## 0.16.0 / 2019-01-17

This release introduces a new API v2, fully generated via the OpenAPI project
[1]. At the same time with this release the previous API v1 is being
deprecated. API v1 will be removed with Alertmanager release v0.18.0.

* [CHANGE] Deprecate API v1
* [CHANGE] Remove `api/v1/alerts/groups` GET endpoint (#1508 & #1525)
* [CHANGE] Revert Alertmanager working directory changes in Docker image back to `/alertmanager` (#1435)
* [CHANGE] Using the recommended label syntax for maintainer in Dockerfile (#1533)
* [CHANGE] Change `alertmanager_notifications_total` to count attempted notifications, not only successful ones (#1578)
* [CHANGE] Run as nobody inside container (#1586)
* [CHANGE] Support `w` for weeks when creating silences, remove `y` for year (#1620)
* [FEATURE] Introduce OpenAPI generated API v2 (#1352)
* [FEATURE] Lookup parts in strings using regexp.MatchString in templates (#1452)
* [FEATURE] Support image/thumb url in attachment in Slack notifier (#1506)
* [FEATURE] Support custom TLS certificates for the email notifier (#1528)
* [FEATURE] Add support for images and links in the PagerDuty notification config (#1559)
* [FEATURE] Add support for grouping by all labels (#1588)
* [FEATURE] [amtool] Add timeout support to amtool commands (#1471)
* [FEATURE] [amtool] Added `config routes` tools for visualization and testing routes (#1511)
* [FEATURE] [amtool] Support adding alerts using amtool (#1461)
* [ENHANCEMENT] Add support for --log.format (#1658)
* [ENHANCEMENT] Add CORS support to API v2 (#1667)
* [ENHANCEMENT] Support HTML, URL title and custom sounds for Pushover (#1634)
* [ENHANCEMENT] Update Alert compact view (#1698)
* [ENHANCEMENT] Support adding custom fields to VictorOps notifications (#1420)
* [ENHANCEMENT] Add help link in UI to Alertmanager documentation (#1522)
* [ENHANCEMENT] Enforce HTTP or HTTPS URLs in Alertmanager config (#1567)
* [ENHANCEMENT] Make OpsGenie API Key a templated string (#1594)
* [ENHANCEMENT] Add name, value and SlackConfirmationField to action in Slack notifier (#1557)
* [ENHANCEMENT] Show more alert information on silence form and silence view pages (#1601)
* [ENHANCEMENT] Add cluster peers DNS refresh job (#1428)
* [BUGFIX] Fix unmarshaling of secret URLs in config (#1663)
* [BUGFIX] Do not write groupbyall and groupby when marshaling config (#1665)
* [BUGFIX] Make a copy of firing alerts with EndsAt=0 when flushing (#1686)
* [BUGFIX] Respect regex matchers when recreating silences in UI (#1697)
* [BUGFIX] Change DefaultGlobalConfig to a function in Alertmanager configuration (#1656)
* [BUGFIX] Fix email template typo in alert-warning style (#1421)
* [BUGFIX] Fix silence redirect on silence creation UI page (#1548)
* [BUGFIX] Add missing `callback_id` parameter in Slack notifier (#1592)
* [BUGFIX] Throw error if no auth mechanism matches in email notifier (#1608)
* [BUGFIX] Use quoted-printable transfer encoding for the email notifier (#1609)
* [BUGFIX] Do not merge expired gossip messages (#1631)
* [BUGFIX] Fix "PLAIN" auth during notification via smtp-over-tls on port 465 (#1591)
* [BUGFIX] [amtool] Support for assuming first label is alertname in silence add and query (#1693)
* [BUGFIX] [amtool] Support assuming first label is alertname in alert query with matchers (#1575)
* [BUGFIX] [amtool] Fix config path check in amtool (#1538)
* [BUGFIX] [amtool] Fix rfc3339 example texts (#1526)
* [BUGFIX] [amtool] Fixed issue with loading path of a default configs (#1529)

[1] https://github.com/prometheus/alertmanager#api

## 0.15.3 / 2018-11-09

* [BUGFIX] Fix alert merging supporting both empty and set EndsAt property for firing alerts send by Prometheus (#1611)

## 0.15.2 / 2018-08-14

* [ENHANCEMENT] [amtool] Add support for stdin to check-config (#1431)
* [ENHANCEMENT] Log PagerDuty v1 response on BadRequest (#1481)
* [BUGFIX] Correctly encode query strings in notifiers (#1516)
* [BUGFIX] Add cache control headers to the API responses to avoid IE caching (#1500)
* [BUGFIX] Avoid listener blocking on unsubscribe (#1482)
* [BUGFIX] Fix a bunch of unhandled errors (#1501)
* [BUGFIX] Update PagerDuty API V2 to send full details on resolve (#1483)
* [BUGFIX] Validate URLs at config load time (#1468)
* [BUGFIX] Fix Settle() interval (#1478)
* [BUGFIX] Fix email to be green if only none firing (#1475)
* [BUGFIX] Handle errors in notify (#1474)
* [BUGFIX] Fix templating of hipchat room id (#1463)

## 0.15.1 / 2018-07-10

* [BUGFIX] Fix email template typo in alert-warning style (#1421)
* [BUGFIX] Fix regression in Pager Duty config (#1455)
* [BUGFIX] Catch templating errors in Wechat Notify (#1436)
* [BUGFIX] Fail when no private address can be found for cluster (#1437)
* [BUGFIX] Make sure we don't miss the first pushPull when joining cluster (#1456)
* [BUGFIX] Fix concurrent read and write group error in dispatch (#1447)

## 0.15.0 / 2018-06-22

* [CHANGE] [amtool] Update silence add and update flags (#1298)
* [CHANGE] Replace deprecated InstrumentHandler() (#1302)
* [CHANGE] Validate Slack field config and only allow the necessary input (#1334)
* [CHANGE] Remove legacy alert ingest endpoint (#1362)
* [CHANGE] Move to memberlist as underlying gossip protocol including cluster flag changes from --mesh.xxx to --cluster.xxx (#1232)
* [CHANGE] Move Alertmanager working directory in Docker image to /etc/alertmanager (#1313)
* [BUGFIX/CHANGE] The default group by is no labels. (#1287)
* [FEATURE] [amtool] Filter alerts by receiver (#1402)
* [FEATURE] Wait for mesh to settle before sending alerts (#1209)
* [FEATURE] [amtool] Support basic auth in alertmanager url (#1279)
* [FEATURE] Make HTTP clients used for integrations configurable
* [ENHANCEMENT] Support receiving alerts with end time and zero start time
* [ENHANCEMENT] Sort dispatched alerts by job+instance (#1234)
* [ENHANCEMENT] Support alert query filters `active` and `unprocessed` (#1366)
* [ENHANCEMENT] [amtool] Expose alert query flags --active and --unprocessed (#1370)
* [ENHANCEMENT] Add Slack actions to notifications (#1355)
* [BUGFIX] Register nflog snapShotSize metric
* [BUGFIX] Sort alerts in correct order before flushing to notifiers (#1349)
* [BUGFIX] Don't reset initial wait timer if flush is in-progress (#1301)
* [BUGFIX] Fix resolved alerts still inhibiting (#1331)
* [BUGFIX] Template wechat config fields (#1356)
* [BUGFIX] Notify resolved alerts properly (#1408)
* [BUGFIX] Fix parsing for label values with commas (#1395)
* [BUGFIX] Hide sensitive Wechat configuration (#1253)
* [BUGFIX] Prepopulate matchers when recreating a silence (#1270)
* [BUGFIX] Fix wechat panic (#1293)
* [BUGFIX] Allow empty matchers in silences/filtering (#1289)
* [BUGFIX] Properly configure HTTP client for Wechat integration

## 0.14.0 / 2018-02-12

* [ENHANCEMENT] [amtool] Silence update support dwy suffixes to expire flag (#1197)
* [ENHANCEMENT] Allow templating PagerDuty receiver severity (#1214)
* [ENHANCEMENT] Include receiver name in failed notifications log messages (#1207)
* [ENHANCEMENT] Allow global opsgenie api key (#1208)
* [ENHANCEMENT] Add mesh metrics (#1225)
* [ENHANCEMENT] Add Class field to PagerDuty; add templating to PagerDuty-CEF fields (#1231)
* [BUGFIX] Don't notify of resolved alerts if none were reported firing (#1198)
* [BUGFIX] Notify only when new firing alerts are added (#1205)
* [BUGFIX] [mesh] Fix pending connections never set to established (#1204)
* [BUGFIX] Allow OpsGenie notifier to have empty team fields (#1224)
* [BUGFIX] Don't count alerts with EndTime in the future as resolved (#1233)
* [BUGFIX] Speed up re-rendering of Silence UI (#1235)
* [BUGFIX] Forbid 0 value for group_interval and repeat_interval (#1230)
* [BUGFIX] Fix WeChat agentid issue (#1229)

## 0.13.0 / 2018-01-12

* [CHANGE] Switch cmd/alertmanager to kingpin (#974)
* [CHANGE] [amtool] Switch amtool to kingpin (#976)
* [CHANGE] [amtool] silence query: --expired flag only shows expired silences (#1190)
* [CHANGE] Return config reload result from reload endpoint (#1180)
* [FEATURE] UI silence form is populated from location bar (#1148)
* [FEATURE] Add /-/healthy endpoint (#1159)
* [ENHANCEMENT] Instrument and log snapshot sizes on maintenance (#1155)
* [ENHANCEMENT] Make alertGC interval configurable (#1151)
* [ENHANCEMENT] Display mesh connections in the Status page (#1164)
* [BUGFIX] Template service keys for pagerduty notifier (#1182)
* [BUGFIX] Fix expire buttons on the silences page (#1171)
* [BUGFIX] Fix JavaScript error in MSIE due to endswith() usage (#1172)
* [BUGFIX] Correctly format UI error output (#1167)

## 0.12.0 / 2017-12-15

* [FEATURE] package amtool in docker container (#1127)
* [FEATURE] Add notify support for Chinese User wechat (#1059)
* [FEATURE] [amtool] Add a new `silence import` command (#1082)
* [FEATURE] [amtool] Add new command to update silence (#1123)
* [FEATURE] [amtool] Add ability to query for silences that will expire soon (#1120)
* [ENHANCEMENT] Template source field in PagerDuty alert payload (#1117)
* [ENHANCEMENT] Add footer field for slack messages (#1141)
* [ENHANCEMENT] Add Slack additional "fields" to notifications (#1135)
* [ENHANCEMENT] Adding check for webhook's URL formatting (#1129)
* [ENHANCEMENT] Let the browser remember the creator of a silence (#1112)
* [BUGFIX] Fix race in stopping inhibitor (#1118)
* [BUGFIX] Fix browser UI when entering negative duration (#1132)

## 0.11.0 / 2017-11-16

* [CHANGE] Make silence negative filtering consistent with alert filtering (#1095)
* [CHANGE] Change HipChat and OpsGenie api config names (#1087)
* [ENHANCEMENT] amtool: Allow 'd', 'w', 'y' time suffixes when creating silence (#1091)
* [ENHANCEMENT] Support OpsGenie Priority field (#1094)
* [BUGFIX] Fix UI when no silences are present (#1090)
* [BUGFIX] Fix OpsGenie Teams field (#1101)
* [BUGFIX] Fix OpsGenie Tags field (#1108)

## 0.10.0 / 2017-11-09

* [CHANGE] Prevent inhibiting alerts in the source of the inhibition (#1017)
* [ENHANCEMENT] Improve amtool check-config use and description text (#1016)
* [ENHANCEMENT] Add metrics about current silences and alerts (#998)
* [ENHANCEMENT] Sorted silences based on current status (#1015)
* [ENHANCEMENT] Add metric of alertmanager position in mesh (#1024)
* [ENHANCEMENT] Initialise notifications_total and notifications_failed_total (#1011)
* [ENHANCEMENT] Allow selectable matchers on silence view (#1030)
* [ENHANCEMENT] Allow template in victorops message_type field (#1038)
* [ENHANCEMENT] Optionally hide inhibited alerts in API response (#1039)
* [ENHANCEMENT] Toggle silenced and inhibited alerts in UI (#1049)
* [ENHANCEMENT] Fix pushover limits (title, message, url) (#1055)
* [ENHANCEMENT] Add limit to OpsGenie message (#1045)
* [ENHANCEMENT] Upgrade OpsGenie notifier to v2 API. (#1061)
* [ENHANCEMENT] Allow template in victorops routing_key field (#1083)
* [ENHANCEMENT] Add support for PagerDuty API v2 (#1054)
* [BUGFIX] Fix inhibit race (#1032)
* [BUGFIX] Fix segfault on amtool (#1031)
* [BUGFIX] Remove .WasInhibited and .WasSilenced fields of Alert type (#1026)
* [BUGFIX] nflog: Fix Log() crash when gossip is nil (#1064)
* [BUGFIX] Fix notifications for flapping alerts (#1071)
* [BUGFIX] Fix shutdown crash with nil mesh router (#1077)
* [BUGFIX] Fix negative matchers filtering (#1077)

## 0.9.1 / 2017-09-29
* [BUGFIX] Fix -web.external-url regression in ui (#1008)
* [BUGFIX] Fix multipart email implementation (#1009)

## 0.9.0 / 2017-09-28
* [ENHANCEMENT] Add current time to webhook message (#909)
* [ENHANCEMENT] Add link_names to slack notifier (#912)
* [ENHANCEMENT] Make ui labels selectable/highlightable (#932)
* [ENHANCEMENT] Make links in ui annotations selectable (#946)
* [ENHANCEMENT] Expose the alert's "fingerprint" (unique identifier) through API (#786)
* [ENHANCEMENT] Add README information for amtool (#939)
* [ENHANCEMENT] Use user-set logging option consistently throughout alertmanager (#968)
* [ENHANCEMENT] Sort alerts returned from API by their fingerprint (#969)
* [ENHANCEMENT] Add edit/delete silence buttons on silence page view (#970)
* [ENHANCEMENT] Add check-config subcommand to amtool (#978)
* [ENHANCEMENT] Add email notification text content support (#934)
* [ENHANCEMENT] Support passing binary name to make build target (#990)
* [ENHANCEMENT] Show total no. of silenced alerts in preview (#994)
* [ENHANCEMENT] Added confirmation dialog when expiring silences (#993)
* [BUGFIX] Fix crash when no mesh router is configured (#919)
* [BUGFIX] Render status page without mesh (#920)
* [BUGFIX] Exit amtool subcommands with non-zero error code (#938)
* [BUGFIX] Change mktemp invocation in makefile to work for macOS (#971)
* [BUGFIX] Add a mutex to silences.go:gossipData (#984)
* [BUGFIX] silences: avoid deadlock (#995)
* [BUGFIX] Ignore expired silences OnGossip (#999)

## 0.8.0 / 2017-07-20

* [FEATURE] Add ability to filter alerts by receiver in the UI (#890)
* [FEATURE] Add User-Agent for webhook requests (#893)
* [ENHANCEMENT] Add possibility to have a global victorops api_key (#897)
* [ENHANCEMENT] Add EntityDisplayName and improve StateMessage for Victorops
  (#769)
* [ENHANCEMENT] Omit empty config fields and show regex upon re-marshaling to
  elide secrets (#864)
* [ENHANCEMENT] Parse API error messages in UI (#866)
* [ENHANCEMENT] Enable sending mail via smtp port 465 (#704)
* [BUGFIX] Prevent duplicate notifications by sorting matchers (#882)
* [BUGFIX] Remove timeout for UI requests (#890)
* [BUGFIX] Update config file location of CLI in flag usage text (#895)

## 0.7.1 / 2017-06-09

* [BUGFIX] Fix filtering by label on Alert list and Silence list page

## 0.7.0 / 2017-06-08

* [CHANGE] Rewrite UI from scratch improving UX
* [CHANGE] Rename `config` to `configYAML` on `api/v1/status`
* [FEATURE] Add ability to update a silence on `api/v1/silences` POST endpoint (See #765)
* [FEATURE] Return alert status on `api/v1/alerts` GET endpoint
* [FEATURE] Serve silence state on `api/v1/silences` GET endpoint
* [FEATURE] Add ability to specify a route prefix
* [FEATURE] Add option to disable AM listening on mesh port
* [ENHANCEMENT] Add ability to specify `filter` string and `silenced` flag on `api/v1/alerts` GET endpoint
* [ENHANCEMENT] Update `cache-control` to prevent caching for web assets in general.
* [ENHANCEMENT] Serve web assets by alertmanager instead of external CDN (See #846)
* [ENHANCEMENT] Elide secrets in alertmanager config (See #840)
* [ENHANCEMENT] AMTool: Move config file to a more consistent location (See #843)
* [BUGFIX] Enable builds for Solaris/Illumos
* [BUGFIX] Load web assets based on url path (See #323)

## 0.6.2 / 2017-05-09

* [BUGFIX] Correctly link to silences from alert again
* [BUGFIX] Correctly hide silenced/show active alerts in UI again
* [BUGFIX] Fix regression of alerts not being displayed until first processing
* [BUGFIX] Fix internal usage of wrong lock for silence markers
* [BUGFIX] Adapt amtool's API parsing to recent API changes
* [BUGFIX] Correctly marshal regexes in config JSON response
* [CHANGE] Anchor silence regex matchers to be consistent with Prometheus
* [ENHANCEMENT] Error if root route is using `continue` keyword

## 0.6.1 / 2017-04-28

* [BUGFIX] Fix incorrectly serialized hash for notification providers.
* [ENHANCEMENT] Add processing status field to alerts.
* [FEATURE] Add config hash metric.

## 0.6.0 / 2017-04-25

* [BUGFIX] Add `groupKey` to `alerts/groups` endpoint https://github.com/prometheus/alertmanager/pull/576
* [BUGFIX] Only notify on firing alerts https://github.com/prometheus/alertmanager/pull/595
* [BUGFIX] Correctly marshal regex's in config for routing tree https://github.com/prometheus/alertmanager/pull/602
* [BUGFIX] Prevent panic when failing to load config https://github.com/prometheus/alertmanager/pull/607
* [BUGFIX] Prevent panic when alertmanager is started with an empty `-mesh.peer` https://github.com/prometheus/alertmanager/pull/726
* [CHANGE] Rename VictorOps config variables https://github.com/prometheus/alertmanager/pull/667
* [CHANGE] No longer generate releases for openbsd/arm https://github.com/prometheus/alertmanager/pull/732
* [ENHANCEMENT] Add `DELETE` as accepted CORS method https://github.com/prometheus/alertmanager/commit/0ecc59076ca6b4cbb63252fa7720a3d89d1c81d3
* [ENHANCEMENT] Switch to using `gogoproto` for protobuf https://github.com/prometheus/alertmanager/pull/715
* [ENHANCEMENT] Include notifier type in logs and errors https://github.com/prometheus/alertmanager/pull/702
* [FEATURE] Expose mesh peers on status page https://github.com/prometheus/alertmanager/pull/644
* [FEATURE] Add `reReplaceAll` template function https://github.com/prometheus/alertmanager/pull/639
* [FEATURE] Allow label-based filtering alerts/silences through API https://github.com/prometheus/alertmanager/pull/633
* [FEATURE] Add commandline tool for interacting with alertmanager https://github.com/prometheus/alertmanager/pull/636

## 0.5.1 / 2016-11-24

* [BUGFIX] Fix crash caused by race condition in silencing
* [ENHANCEMENT] Improve logging of API errors
* [ENHANCEMENT] Add metrics for the notification log

## 0.5.0 / 2016-11-01

This release requires a storage wipe. It contains fundamental internal
changes that came with implementing the high availability mode.

* [FEATURE] Alertmanager clustering for high availability
* [FEATURE] Garbage collection of old silences and notification logs
* [CHANGE] New storage format
* [CHANGE] Stricter silence semantics for consistent historical view

## 0.4.2 / 2016-09-02

* [BUGFIX] Fix broken regex checkbox in silence form
* [BUGFIX] Simplify inconsistent silence update behavior

## 0.4.1 / 2016-08-31

* [BUGFIX] Wait for silence query to finish instead of showing error
* [BUGFIX] Fix sorting of silences
* [BUGFIX] Provide visual feedback after creating a silence
* [BUGFIX] Fix styling of silences
* [ENHANCEMENT] Provide cleaner API silence interface

## 0.4.0 / 2016-08-23

* [FEATURE] Silences are now paginated in the web ui
* [CHANGE] Failure to start on unparsed flags

## 0.3.0 / 2016-07-07

* [CHANGE] Alerts are purely in memory and no longer persistent across restarts
* [FEATURE] Add SMTP LOGIN authentication mechanism

## 0.2.1 / 2016-06-23

* [ENHANCEMENT] Allow inheritance of route receiver
* [ENHANCEMENT] Add silence cache to silence provider
* [BUGFIX] Fix HipChat room number in integration URL

## 0.2.0 / 2016-06-17

This release uses a new storage backend based on BoltDB. You have to backup
and wipe your former storage path to run it.

* [CHANGE] Use BoltDB as data store.
* [CHANGE] Move SMTP authentication to configuration file
* [FEATURE] add /-/reload HTTP endpoint
* [FEATURE] Filter silenced alerts in web UI
* [ENHANCEMENT] reduce inhibition computation complexity
* [ENHANCEMENT] Add support for teams and tags in OpsGenie integration
* [BUGFIX] Handle OpsGenie responses correctly
* [BUGFIX] Fix Pushover queue length issue
* [BUGFIX] STARTTLS before querying auth mechanism in email integration

## 0.1.1 / 2016-03-15
* [BUGFIX] Fix global database lock issue
* [ENHANCEMENT] Improve SQLite alerts index
* [ENHANCEMENT] Enable debug endpoint

## 0.1.0 / 2016-02-23
This version is a full rewrite of the Alertmanager with a very different
feature set. Thus, there is no meaningful changelog.

Changes with respect to 0.1.0-beta2:
* [CHANGE] Expose same data structure to templates and webhook
* [ENHANCEMENT] Show generator URL in default templates and web UI
* [ENHANCEMENT] Support for Slack icon_emoji field
* [ENHANCEMENT] Expose incident key to templates and webhook data
* [ENHANCEMENT] Allow markdown in Slack 'text' field
* [BUGFIX] Fixed database locking issue

## 0.1.0-beta2 / 2016-02-03
* [BUGFIX] Properly set timeout for incoming alerts with fixed start time
* [ENHANCEMENT] Send source field in OpsGenie integration
* [ENHANCEMENT] Improved routing configuration validation
* [FEATURE] Basic instrumentation added

## 0.1.0-beta1 / 2016-01-08
* [BUGFIX] Send full alert group state on each update. Fixes erroneous resolved notifications.
* [FEATURE] HipChat integration
* [CHANGE] Slack integration no longer sends resolved notifications by default

## 0.1.0-beta0 / 2015-12-23
This version is a full rewrite of the Alertmanager with a very different
feature set. Thus, there is no meaningful changelog.

## 0.0.4 / 2015-09-09
* [BUGFIX] Fix version info string in startup message.
* [BUGFIX] Fix Pushover notifications by setting the right priority level, as
  well as required retry and expiry intervals.
* [FEATURE] Make it possible to link to individual alerts in the UI.
* [FEATURE] Rearrange alert columns in UI and allow expanding more alert details.
* [FEATURE] Add Amazon SNS notifications.
* [FEATURE] Add OpsGenie Webhook notifications.
* [FEATURE] Add `-web.external-url` flag to control the externally visible
  Alertmanager URL.
* [FEATURE] Add runbook and alertmanager URLs to PagerDuty and email notifications.
* [FEATURE] Add a GET API to /api/alerts which pulls JSON formatted
  AlertAggregates.
* [ENHANCEMENT] Sort alerts consistently in web UI.
* [ENHANCEMENT] Suggest to use email address as silence creator.
* [ENHANCEMENT] Make Slack timeout configurable.
* [ENHANCEMENT] Add channel name to error logging about Slack notifications.
* [ENHANCEMENT] Refactoring and tests for Flowdock notifications.
* [ENHANCEMENT] New Dockerfile using alpine-golang-make-onbuild base image.
* [CLEANUP] Add Docker instructions and other cleanups in README.md.
* [CLEANUP] Update Makefile.COMMON from prometheus/utils.

## 0.0.3 / 2015-06-10
* [BUGFIX] Fix email template body writer being called with parameters in wrong order.

## 0.0.2 / 2015-06-09

* [BUGFIX] Fixed silences.json permissions in Docker image.
* [CHANGE] Changed case of API JSON properties to initial lower letter.
* [CHANGE] Migrated logging to use http://github.com/prometheus/log.
* [FEATURE] Flowdock notification support.
* [FEATURE] Slack notification support.
* [FEATURE] Generic webhook notification support.
* [FEATURE] Support for "@"-mentions in HipChat notifications.
* [FEATURE] Path prefix option to support reverse proxies.
* [ENHANCEMENT] Improved web redirection and 404 behavior.
* [CLEANUP] Updated compiled web assets from source.
* [CLEANUP] Updated fsnotify package to its new source location.
* [CLEANUP] Updates to README.md and AUTHORS.md.
* [CLEANUP] Various smaller cleanups and improvements.
