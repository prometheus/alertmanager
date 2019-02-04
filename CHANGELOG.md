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
* [FEATURE] [amtool] Added `config routes` tools for vizualization and testing routes (#1511)
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
* [BUGFIX] Fix unmarshalling of secret URLs in config (#1663)
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
* [ENHANCEMENT] Omit empty config fields and show regex upon re-marshalling to
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
* [CHANGE] Move SMTP authentification to configuration file
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
