---
title: Configuration
sort_rank: 3
nav_icon: sliders
---

[Alertmanager](https://github.com/prometheus/alertmanager) is configured via
command-line flags and a configuration file.
While the command-line flags configure immutable system parameters, the
configuration file defines inhibition rules, notification routing and
notification receivers.

The [visual editor](https://www.prometheus.io/webtools/alerting/routing-tree-editor)
can assist in building routing trees.

To view all available command-line flags, run `alertmanager -h`.

Alertmanager can reload its configuration at runtime. If the new configuration
is not well-formed, the changes will not be applied and an error is logged.
A configuration reload is triggered by sending a `SIGHUP` to the process or
sending an HTTP POST request to the `/-/reload` endpoint.

## Limits

Alertmanager supports a number of configurable limits via command-line flags.

To limit the maximum number of silences, including expired ones,
use the `--silences.max-silences` flag.
You can limit the maximum size of individual silences with `--silences.max-per-silence-bytes`,
where the unit is in bytes.

Both limits are disabled by default.

## Configuration file introduction

To specify which configuration file to load, use the `--config.file` flag.

```bash
./alertmanager --config.file=alertmanager.yml
```

The file is written in the [YAML format](http://en.wikipedia.org/wiki/YAML),
defined by the scheme described below.
Brackets indicate that a parameter is optional. For non-list parameters the
value is set to the specified default.

Generic placeholders are defined as follows:

* `<duration>`: a duration matching the regular expression `((([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?|0)`, e.g. `1d`, `1h30m`, `5m`, `10s`
* `<labelname>`: a string matching the regular expression `[a-zA-Z_][a-zA-Z0-9_]*`
* `<labelvalue>`: a string of unicode characters
* `<filepath>`: a valid path in the current working directory
* `<boolean>`: a boolean that can take the values `true` or `false`
* `<string>`: a regular string
* `<secret>`: a regular string that is a secret, such as a password
* `<tmpl_string>`: a string which is template-expanded before usage
* `<tmpl_secret>`: a string which is template-expanded before usage that is a secret
* `<int>`: an integer value
* `<regex>`: any valid [RE2 regular expression](https://github.com/google/re2/wiki/Syntax) (The regex is anchored on both ends. To un-anchor the regex, use `.*<regex>.*`.)

The other placeholders are specified separately.

A provided [valid example file](https://github.com/prometheus/alertmanager/blob/main/doc/examples/simple.yml)
shows usage in context.

## File layout and global settings

The global configuration specifies parameters that are valid in all other
configuration contexts. They also serve as defaults for other configuration
sections. The other top-level sections are documented below on this page.

```yaml
global:
  # The default SMTP From header field.
  [ smtp_from: <tmpl_string> ]
  # The default SMTP smarthost used for sending emails, including port number.
  # Port number usually is 25, or 587 for SMTP over TLS (sometimes referred to as STARTTLS).
  # Example: smtp.example.org:587
  [ smtp_smarthost: <string> ]
  # The default hostname to identify to the SMTP server.
  [ smtp_hello: <string> | default = "localhost" ]
  # SMTP Auth using CRAM-MD5, LOGIN and PLAIN. If empty, Alertmanager doesn't authenticate to the SMTP server.
  [ smtp_auth_username: <string> ]
  # SMTP Auth using LOGIN and PLAIN.
  [ smtp_auth_password: <secret> ]
  # SMTP Auth using LOGIN and PLAIN.
  [ smtp_auth_password_file: <string> ]
  # SMTP Auth using PLAIN.
  [ smtp_auth_identity: <string> ]
  # SMTP Auth using CRAM-MD5.
  [ smtp_auth_secret: <secret> ]
  # The default SMTP TLS requirement.
  # Note that Go does not support unencrypted connections to remote SMTP endpoints.
  [ smtp_require_tls: <bool> | default = true ]
  # The default TLS configuration for SMTP receivers
  [ smtp_tls_config: <tls_config> ]

  # Default settings for the JIRA integration.
  [ jira_api_url: <string> ]

  # The API URL to use for Slack notifications.
  [ slack_api_url: <secret> ]
  [ slack_api_url_file: <filepath> ]
  [ victorops_api_key: <secret> ]
  [ victorops_api_key_file: <filepath> ]
  [ victorops_api_url: <string> | default = "https://alert.victorops.com/integrations/generic/20131114/alert/" ]
  [ pagerduty_url: <string> | default = "https://events.pagerduty.com/v2/enqueue" ]
  [ opsgenie_api_key: <secret> ]
  [ opsgenie_api_key_file: <filepath> ]
  [ opsgenie_api_url: <string> | default = "https://api.opsgenie.com/" ]
  [ rocketchat_api_url: <string> | default = "https://open.rocket.chat/" ]
  [ rocketchat_token: <secret> ]
  [ rocketchat_token_file: <filepath> ]
  [ rocketchat_token_id: <secret> ]
  [ rocketchat_token_id_file: <filepath> ]
  [ wechat_api_url: <string> | default = "https://qyapi.weixin.qq.com/cgi-bin/" ]
  [ wechat_api_secret: <secret> ]
  [ wechat_api_corp_id: <string> ]
  [ telegram_api_url: <string> | default = "https://api.telegram.org" ]
  [ webex_api_url: <string> | default = "https://webexapis.com/v1/messages" ]
  # The default HTTP client configuration
  [ http_config: <http_config> ]

  # ResolveTimeout is the default value used by alertmanager if the alert does
  # not include EndsAt, after this time passes it can declare the alert as resolved if it has not been updated.
  # This has no impact on alerts from Prometheus, as they always include EndsAt.
  [ resolve_timeout: <duration> | default = 5m ]

# Files from which custom notification template definitions are read.
# The last component may use a wildcard matcher, e.g. 'templates/*.tmpl'.
templates:
  [ - <filepath> ... ]

# The root node of the routing tree.
route: <route>

# A list of notification receivers.
receivers:
  - <receiver> ...

# A list of inhibition rules.
inhibit_rules:
  [ - <inhibit_rule> ... ]

# DEPRECATED: use time_intervals below.
# A list of mute time intervals for muting routes.
mute_time_intervals:
  [ - <mute_time_interval> ... ]

# A list of time intervals for muting/activating routes.
time_intervals:
  [ - <time_interval> ... ]
```

## Route-related settings

Routing-related settings allow configuring how alerts are routed, aggregated, throttled, and muted based on time.

### `<route>`

A route block defines a node in a routing tree and its children. Its optional
configuration parameters are inherited from its parent node if not set.

Every alert enters the routing tree at the configured top-level route, which
must match all alerts (i.e. not have any configured matchers).
It then traverses the child nodes. If `continue` is set to false, it stops
after the first matching child. If `continue` is true on a matching node, the
alert will continue matching against subsequent siblings.
If an alert does not match any children of a node (no matching child nodes, or
none exist), the alert is handled based on the configuration parameters of the
current node.

See [Alertmanager concepts](https://prometheus.io/docs/alerting/alertmanager/#grouping) for more information on grouping.

```yaml
[ receiver: <string> ]
# The labels by which incoming alerts are grouped together. For example,
# multiple alerts coming in for cluster=A and alertname=LatencyHigh would
# be batched into a single group.
#
# To aggregate by all possible labels use the special value '...' as the sole label name, for example:
# group_by: ['...']
# This effectively disables aggregation entirely, passing through all
# alerts as-is. This is unlikely to be what you want, unless you have
# a very low alert volume or your upstream notification system performs
# its own grouping.
[ group_by: '[' <labelname>, ... ']' ]

# Whether an alert should continue matching subsequent sibling nodes.
[ continue: <boolean> | default = false ]

# DEPRECATED: Use matchers below.
# A set of equality matchers an alert has to fulfill to match the node.
match:
  [ <labelname>: <labelvalue>, ... ]

# DEPRECATED: Use matchers below.
# A set of regex-matchers an alert has to fulfill to match the node.
match_re:
  [ <labelname>: <regex>, ... ]

# A list of matchers that an alert has to fulfill to match the node.
matchers:
  [ - <matcher> ... ]

# How long to wait before sending the first notification for a new group of
# alerts. Allows to wait for alerts to arrive from other rule groups or
# Prometheus servers, and for one or more inhibiting alerts to arrive and mute
# any target alerts before the first notification.
#
# A short group_wait will reduce the time to wait before sending the first
# notification for a new group of alerts. However, if group_wait is too short
# then the first notification might not contain the complete set of expected
# alerts, and alerts that should be inhibited might not be inhibited if the
# inhibiting alerts have not arrived in time.
#
# A long group_wait will increase the time to wait before sending the first
# notification for a new group of alerts. However, if group_wait is too long
# then notifications for firing alerts might not be sent within a reasonable
# time.
#
# If an alert is resolved before group_wait has elapsed, no notification will
# be sent for that alert. This reduces noise of flapping alerts.

# A notification for any alerts that missed the initial group_wait will be
# sent at the next group_interval instead.
#
# If omitted, child routes inherit the group_wait of the parent route.
[ group_wait: <duration> | default = 30s ]

# How long to wait before sending subsequent notifications for an existing
# group of alerts after group_wait.
#
# The group_interval is a recurring timer that starts as soon as group_wait
# has elapsed. At each group_interval, Alertmanager checks if any new alerts
# have fired or any firing alerts have resolved since the last group_interval,
# and if they have a notification is sent. If they haven't, Alertmanager checks
# if the repeat_interval has elapsed instead.
#
# If omitted, child routes inherit the group_interval of the parent route.
[ group_interval: <duration> | default = 5m ]

# How long to wait before repeating the last notification. Notifications are
# not repeated if any new alerts have fired or any firing alerts have resolved
# since the last group_interval.
#
# Since the repeat_interval is checked after each group_interval, it should
# be a multiple of the group_interval. If it's not, the repeat_interval
# is rounded up to the next multiple of the group_interval.
#
# In addition, if repeat_interval is longer then `--data.retention`, the
# notification will be repeated at the end of the data retention period
# instead.
#
# If omitted, child routes inherit the repeat_interval of the parent route.
[ repeat_interval: <duration> | default = 4h ]

# Times when the route should be muted. These must match the name of a
# time interval defined in the time_intervals section.
# Additionally, the root node cannot have any mute times.
# When a route is muted it will not send any notifications, but
# otherwise acts normally (including ending the route-matching process
# if the `continue` option is not set.)
mute_time_intervals:
  [ - <string> ...]

# Times when the route should be active. These must match the name of a
# time interval defined in the time_intervals section. An empty value
# means that the route is always active.
# Additionally, the root node cannot have any active times.
# The route will send notifications only when active, but otherwise
# acts normally (including ending the route-matching process
# if the `continue` option is not set).
active_time_intervals:
  [ - <string> ...]

# Zero or more child routes.
routes:
  [ - <route> ... ]
```

#### Example

```yaml
# The root route with all parameters, which are inherited by the child
# routes if they are not overwritten.
route:
  receiver: 'default-receiver'
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  group_by: [cluster, alertname]
  # All alerts that do not match the following child routes
  # will remain at the root node and be dispatched to 'default-receiver'.
  routes:
  # All alerts with service=mysql or service=cassandra
  # are dispatched to the database pager.
  - receiver: 'database-pager'
    group_wait: 10s
    matchers:
    - service=~"mysql|cassandra"
  # All alerts with the team=frontend label match this sub-route.
  # They are grouped by product and environment rather than cluster
  # and alertname.
  - receiver: 'frontend-pager'
    group_by: [product, environment]
    matchers:
    - team="frontend"

  # All alerts with the service=inhouse-service label match this sub-route.
  # the route will be muted during offhours and holidays time intervals.
  # even if it matches, it will continue to the next sub-route
  - receiver: 'dev-pager'
    matchers:
      - service="inhouse-service"
    mute_time_intervals:
      - offhours
      - holidays
    continue: true

    # All alerts with the service=inhouse-service label match this sub-route
    # the route will be active only during offhours and holidays time intervals.
  - receiver: 'on-call-pager'
    matchers:
      - service="inhouse-service"
    active_time_intervals:
      - offhours
      - holidays
```

### `<time_interval>`

A `time_interval` specifies a named interval of time that may be referenced
in the routing tree to mute/activate particular routes for particular times of the day.

```yaml
name: <string>
time_intervals:
  [ - <time_interval_spec> ... ]
```

#### `<time_interval_spec>`

A `time_interval_spec` contains the actual definition for an interval of time. The syntax
supports the following fields:

```yaml
- times:
  [ - <time_range> ...]
  weekdays:
  [ - <weekday_range> ...]
  days_of_month:
  [ - <days_of_month_range> ...]
  months:
  [ - <month_range> ...]
  years:
  [ - <year_range> ...]
  location: <string>
```

All fields are lists. Within each non-empty list, at least one element must be satisfied to match
the field. If a field is left unspecified, any value will match the field. For an instant of time
to match a complete time interval, all fields must match.
Some fields support ranges and negative indices, and are detailed below. If a time zone is not
specified, then the times are taken to be in UTC.

`time_range`: Ranges inclusive of the starting time and exclusive of the end time to
make it easy to represent times that start/end on hour boundaries.
For example, `start_time: '17:00'` and `end_time: '24:00'` will begin at 17:00 and finish
immediately before 24:00. They are specified like so:

```yaml
times:
- start_time: HH:MM
  end_time: HH:MM
```

`weekday_range`: A list of days of the week, where the week begins on Sunday and ends on Saturday.
Days should be specified by name (e.g. 'Sunday'). For convenience, ranges are also accepted
of the form `<start_day>:<end_day>` and are inclusive on both ends. For example:
`['monday:wednesday','saturday', 'sunday']`

`days_of_month_range`: A list of numerical days in the month. Days begin at 1.
Negative values are also accepted which begin at the end of the month,
e.g. -1 during January would represent January 31. For example: `['1:5', '-3:-1']`.
Extending past the start or end of the month will cause it to be clamped. E.g. specifying
`['1:31']` during February will clamp the actual end date to 28 or 29 depending on leap years.
Inclusive on both ends.

`month_range`: A list of calendar months identified by a case-insensitive name (e.g. 'January') or by number,
where January = 1. Ranges are also accepted. For example, `['1:3', 'may:august', 'december']`.
Inclusive on both ends.

`year_range`: A numerical list of years. Ranges are accepted. For example, `['2020:2022', '2030']`.
Inclusive on both ends.

`location`: A string that matches a location in the IANA time zone database. For
example, `'Australia/Sydney'`. The location provides the time zone for the time
interval. For example, a time interval with a location of `'Australia/Sydney'` that
contained something like:

```yaml
times:
- start_time: 09:00
  end_time: 17:00
weekdays: ['monday:friday']
```

would include any time that fell between the hours of 9:00AM and 5:00PM, between Monday
and Friday, using the local time in Sydney, Australia.

You may also use `'Local'` as a location to use the local time of the machine where
Alertmanager is running, or `'UTC'` for UTC time. If no timezone is provided, the time
interval is taken to be in UTC time.**Note:** On Windows, only `Local` or `UTC` are
supported unless you provide a custom time zone database using the `ZONEINFO`
environment variable.

## Inhibition-related settings

Inhibition allows muting a set of alerts based on the presence of another set of
alerts. This allows establishing dependencies between systems or services such that
only the most relevant of a set of interconnected alerts are sent out during an outage.

See [Alertmanager concepts](https://prometheus.io/docs/alerting/alertmanager/#inhibition) for more information on inhibition.

### `<inhibit_rule>`

An inhibition rule mutes an alert (target) matching a set of matchers
when an alert (source) exists that matches another set of matchers.
Both target and source alerts must have the same label values
for the label names in the `equal` list.

Semantically, a missing label and a label with an empty value are the same
thing. Therefore, if all the label names listed in `equal` are missing from
both the source and target alerts, the inhibition rule will apply.

To prevent an alert from inhibiting itself, an alert that matches _both_ the
target and the source side of a rule cannot be inhibited by alerts for which
the same is true (including itself). However, we recommend to choose target and
source matchers in a way that alerts never match both sides. It is much easier
to reason about and does not trigger this special case.

```yaml
# DEPRECATED: Use target_matchers below.
# Matchers that have to be fulfilled in the alerts to be muted.
target_match:
  [ <labelname>: <labelvalue>, ... ]
# DEPRECATED: Use target_matchers below.
target_match_re:
  [ <labelname>: <regex>, ... ]

# A list of matchers that have to be fulfilled by the target
# alerts to be muted.
target_matchers:
  [ - <matcher> ... ]

# DEPRECATED: Use source_matchers below.
# Matchers for which one or more alerts have to exist for the
# inhibition to take effect.
source_match:
  [ <labelname>: <labelvalue>, ... ]
# DEPRECATED: Use source_matchers below.
source_match_re:
  [ <labelname>: <regex>, ... ]

# A list of matchers for which one or more alerts have
# to exist for the inhibition to take effect.
source_matchers:
  [ - <matcher> ... ]

# Labels that must have an equal value in the source and target
# alert for the inhibition to take effect.
[ equal: '[' <labelname>, ... ']' ]
```

## Label matchers

Label matchers match alerts to routes, silences, and inhibition rules.

**Important**: Prometheus is adding support for UTF-8 in labels and metrics. In order to also support UTF-8 in the Alertmanager, Alertmanager versions 0.27 and later have a new parser for matchers that has a number of backwards incompatible changes. While most matchers will be forward-compatible, some will not. Alertmanager is operating a transition period where it supports both UTF-8 and classic matchers, and has provided a number of tools to help you prepare for the transition.

If this is a new Alertmanager installation, we recommend enabling UTF-8 strict mode before creating an Alertmanager configuration file. You can find instructions on how to enable UTF-8 strict mode [here](#utf-8-strict-mode).

If this is an existing Alertmanager installation, we recommend running the Alertmanager in the default mode called fallback mode before enabling UTF-8 strict mode. In this mode, Alertmanager will log a warning if you need to make any changes to your configuration file before UTF-8 strict mode can be enabled. Alertmanager will make UTF-8 strict mode the default in the next two versions, so it's important to transition as soon as possible.

Irrespective of whether an Alertmanager installation is a new or existing installation, you can also use `amtool` to validate that an Alertmanager configuration file is compatible with UTF-8 strict mode before enabling it in Alertmanager server. You do not need a running Alertmanager server to do this. You can find instructions on how to validate an Alertmanager configuration file using `amtool` [here](#verification).

### Alertmanager server operational modes

During the transition period, Alertmanager supports three modes of operation. These are known as fallback mode, UTF-8 strict mode and classic mode. Fallback mode is the default mode.

Operators of Alertmanager servers should transition to UTF-8 strict mode before the end of the transition period. Alertmanager will make UTF-8 strict mode the default in the next two versions, so it's important to transition as soon as possible.

#### Fallback mode

Alertmanager runs in a special mode called fallback mode as its default mode. As operators, you should not experience any difference in how your routes, silences or inhibition rules work.

In fallback mode, configurations are first parsed as UTF-8 matchers, and if incompatible with the UTF-8 parser, are then parsed as classic matchers. If your Alertmanager configuration contains matchers that are incompatible with the UTF-8 parser, Alertmanager will parse them as classic matchers and log a warning. This warning also includes a suggestion on how to change the matchers from classic matchers to UTF-8 matchers. For example:

```
ts=2024-02-11T10:00:00Z caller=parse.go:176 level=warn msg="Alertmanager is moving to a new parser for labels and matchers, and this input is incompatible. Alertmanager has instead parsed the input using the classic matchers parser as a fallback. To make this input compatible with the UTF-8 matchers parser please make sure all regular expressions and values are double-quoted and backslashes are escaped. If you are still seeing this message please open an issue." input="foo=" origin=config err="end of input: expected label value" suggestion="foo=\"\""
```

Here the matcher `foo=` can be made into a valid UTF-8 matcher by double quoting the right hand side of the expression to give `foo=""`. These two matchers are equivalent, however with UTF-8 matchers the right hand side of the matcher is a required field.

In rare cases, a configuration can cause disagreement between the UTF-8 and classic parser. This happens when a matcher is valid in both parsers, but due to added support for UTF-8, results in different parsings depending on which parser is used. If your Alertmanager configuration has disagreement, Alertmanager will use the classic parser and log a warning. For example:

```
ts=2024-02-11T10:00:00Z caller=parse.go:183 level=warn msg="Matchers input has disagreement" input="qux=\"\\xf0\\x9f\\x99\\x82\"\n" origin=config
```

Any occurrences of disagreement should be looked at on a case by case basis as depending on the nature of the disagreement, the configuration might not need updating before enabling UTF-8 strict mode. For example `\xf0\x9f\x99\x82` is the byte sequence for the 🙂 emoji. If the intention is to match a literal 🙂 emoji then no change is required. However, if the intention is to match the literal `\xf0\x9f\x99\x82` then the matcher should be changed to `qux="\\xf0\\x9f\\x99\\x82"`.

#### UTF-8 strict mode

In UTF-8 strict mode, Alertmanager disables support for classic matchers:

```bash
alertmanager --config.file=config.yml --enable-feature="utf8-strict-mode"
```

This mode should be enabled for new Alertmanager installations, and existing Alertmanager installations once all warnings of incompatible matchers have been resolved. Alertmanager will not start in UTF-8 strict mode until all the warnings of incompatible matchers have been resolved:

```
ts=2024-02-11T10:00:00Z caller=coordinator.go:118 level=error component=configuration msg="Loading configuration file failed" file=config.yml err="end of input: expected label value"
```

UTF-8 strict mode will be the default mode of Alertmanager at the end of the transition period.

#### Classic mode

Classic mode is equivalent to Alertmanager versions 0.26.0 and older:

```bash
alertmanager --config.file=config.yml --enable-feature="classic-mode"
```

You can use this mode if you suspect there is an issue with fallback mode or UTF-8 strict mode. In such cases, please open an issue on GitHub with as much information as possible.

### Verification

You can use `amtool` to validate that an Alertmanager configuration file is compatible with UTF-8 strict mode before enabling it in Alertmanager server. You do not need a running Alertmanager server to do this.

Just like Alertmanager server, `amtool` will log a warning if the configuration is incompatible or contains disagreement:

```
amtool check-config config.yml
Checking 'config.yml'
level=warn msg="Alertmanager is moving to a new parser for labels and matchers, and this input is incompatible. Alertmanager has instead parsed the input using the classic matchers parser as a fallback. To make this input compatible with the UTF-8 matchers parser please make sure all regular expressions and values are double-quoted and backslashes are escaped. If you are still seeing this message please open an issue." input="foo=" origin=config err="end of input: expected label value" suggestion="foo=\"\""
level=warn msg="Matchers input has disagreement" input="qux=\"\\xf0\\x9f\\x99\\x82\"\n" origin=config
  SUCCESS
Found:
 - global config
 - route
 - 2 inhibit rules
 - 2 receivers
 - 0 templates
```

You will know if a configuration is compatible with UTF-8 strict mode when no warnings are logged in `amtool`:

```
amtool check-config config.yml
Checking 'config.yml'  SUCCESS
Found:
 - global config
 - route
 - 2 inhibit rules
 - 2 receivers
 - 0 templates
```

You can also use `amtool` in UTF-8 strict mode as an additional level of verification. You will know that a configuration is invalid because the command will fail:

```
amtool check-config config.yml --enable-feature="utf8-strict-mode"
level=warn msg="UTF-8 mode enabled"
Checking 'config.yml'  FAILED: end of input: expected label value

amtool: error: failed to validate 1 file(s)
```

You will know that a configuration is valid because the command will succeed:

```
amtool check-config config.yml --enable-feature="utf8-strict-mode"
level=warn msg="UTF-8 mode enabled"
Checking 'config.yml'  SUCCESS
Found:
 - global config
 - route
 - 2 inhibit rules
 - 2 receivers
 - 0 templates
```

### `<matcher>`

#### UTF-8 matchers

A UTF-8 matcher consists of three tokens:

- An unquoted literal or a double-quoted string for the label name.
- One of `=`, `!=`, `=~`, or `!~`. `=` means equals, `!=` means not equal, `=~` means matches the regular expression and `!~` means doesn't match the regular expression.
- An unquoted literal or a double-quoted string for the regular expression or label value.

Unquoted literals can contain all UTF-8 characters other than the reserved characters. The reserved characters include whitespace and all characters in ``` { } ! = ~ , \ " ' ` ```. For example, `foo`, `[a-zA-Z]+`, and `Προμηθεύς` (Prometheus in Greek) are all examples of valid unquoted literals. However, `foo!` is not a valid literal as `!` is a reserved character.

Double-quoted strings can contain all UTF-8 characters. Unlike unquoted literals, there are no reserved characters. However, literal double quotes and backslashes must be escaped with a single backslash. For example, to match the regular expression `\d+` the backslash must be escaped `"\\d+"`. This is because double-quoted strings follow the same rules as Go's [string literals](https://go.dev/ref/spec#String_literals). Double-quoted strings also support UTF-8 code points. For example, `"foo!"`, `"bar,baz"`, `"\"baz qux\""` and `"\xf0\x9f\x99\x82"`.

#### Classic matchers

A classic matcher is a string with a syntax inspired by PromQL and OpenMetrics. The syntax of a classic matcher consists of three tokens:

- A valid Prometheus label name.
- One of `=`, `!=`, `=~`, or `!~`. `=` means equals, `!=` means that the strings are not equal, `=~` is used for equality of regex expressions and `!~` is used for un-equality of regex expressions. They have the same meaning as known from PromQL selectors.
- A UTF-8 string, which may be enclosed in double quotes. Before or after each token, there may be any amount of whitespace.

The 3rd token may be the empty string. Within the 3rd token, OpenMetrics escaping rules apply: `\"` for a double-quote, `\n` for a line feed, `\\` for a literal backslash. Unescaped `"` must not occur inside the 3rd token (only as the 1st or last character). However, literal line feed characters are tolerated, as are single `\` characters not followed by `\`, `n`, or `"`. They act as a literal backslash in that case.

#### Composition of matchers

You can compose matchers to create complex match expressions. When composed, all matchers must match for the entire expression to match. For example, the expression `{alertname="Watchdog", severity=~"warning|critical"}` will match an alert with labels `alertname=Watchdog, severity=critical` but not an alert with labels `alertname=Watchdog, severity=none` as while the alertname is Watchdog the severity is neither warning nor critical.

You can compose matchers into expressions with a YAML list:

```yaml
matchers:
  - alertname = Watchdog
  - severity =~ "warning|critical"
```

or as a PromQL inspired expression where each matcher is comma separated:

```
{alertname="Watchdog", severity=~"warning|critical"}
```

A single trailing comma is permitted:

```
{alertname="Watchdog", severity=~"warning|critical",}
```

The open `{` and close `}` brace are optional:

```
alertname="Watchdog", severity=~"warning|critical"
```

However, both must be either present or omitted. You cannot have incomplete open or close braces:

```
{alertname="Watchdog", severity=~"warning|critical"
```

```
alertname="Watchdog", severity=~"warning|critical"}
```

You cannot have duplicate open or close braces either:

```
{{alertname="Watchdog", severity=~"warning|critical",}}
```

Whitespace (spaces, tabs and newlines) is permitted outside double quotes and has no effect on the matchers themselves. For example:

```
{
   alertname = "Watchdog",
   severity =~ "warning|critical",
}
```

is equivalent to:

```
{alertname="Watchdog",severity=~"warning|critical"}
```

#### More examples

Here are some more examples:

1. Two equals matchers composed as a YAML list:

    ```yaml
    matchers:
      - foo = bar
      - dings != bums
    ```

2. Two matchers combined composed as a short-form YAML list:

    ```yaml
    matchers: [ foo = bar, dings != bums ]
    ```

   As shown below, in the short-form, it's better to use double quotes to avoid problems with special characters like commas:

   ```yaml
   matchers: [ "foo = \"bar,baz\"", "dings != bums" ]
   ```

3. You can also put both matchers into one PromQL-like string. Single quotes work best here:

    ```yaml
    matchers: [ '{foo="bar", dings!="bums"}' ]
    ```

4. To avoid issues with escaping and quoting rules in YAML, you can also use a YAML block:

    ```yaml
    matchers:
      - |
          {quote=~"She said: \"Hi, all!( How're you…)?\""}
    ```

## General receiver-related settings

These receiver settings allow configuring notification destinations (receivers) and HTTP client options for HTTP-based receivers.

### `<receiver>`

Receiver is a named configuration of one or more notification integrations.

Note: As part of lifting the past moratorium on new receivers it was agreed that, in addition to the existing requirements, new notification integrations will be required to have a committed maintainer with push access.

```yaml
# The unique name of the receiver.
name: <string>

# Configurations for several notification integrations.
discord_configs:
  [ - <discord_config>, ... ]
email_configs:
  [ - <email_config>, ... ]
msteams_configs:
  [ - <msteams_config>, ... ]
msteamsv2_configs:
  [ - <msteamsv2_config>, ... ]
jira_configs:
  [ - <jira_config>, ... ]
opsgenie_configs:
  [ - <opsgenie_config>, ... ]
pagerduty_configs:
  [ - <pagerduty_config>, ... ]
pushover_configs:
  [ - <pushover_config>, ... ]
rocketchat_configs:
  [ - <rocketchat_config>, ... ]
slack_configs:
  [ - <slack_config>, ... ]
sns_configs:
  [ - <sns_config>, ... ]
telegram_configs:
  [ - <telegram_config>, ... ]
victorops_configs:
  [ - <victorops_config>, ... ]
webex_configs:
  [ - <webex_config>, ... ]
webhook_configs:
  [ - <webhook_config>, ... ]
wechat_configs:
  [ - <wechat_config>, ... ]
```

### `<http_config>`

An `http_config` allows configuring the HTTP client that the receiver uses to
communicate with HTTP-based API services.

```yaml
# Note that `basic_auth` and `authorization` options are mutually exclusive.

# Sets the `Authorization` header with the configured username and password.
# password and password_file are mutually exclusive.
basic_auth:
  [ username: <string> ]
  [ password: <secret> ]
  [ password_file: <string> ]

# Optional the `Authorization` header configuration.
authorization:
  # Sets the authentication type.
  [ type: <string> | default: Bearer ]
  # Sets the credentials. It is mutually exclusive with
  # `credentials_file`.
  [ credentials: <secret> ]
  # Sets the credentials with the credentials read from the configured file.
  # It is mutually exclusive with `credentials`.
  [ credentials_file: <filename> ]

# Optional OAuth 2.0 configuration.
# Cannot be used at the same time as basic_auth or authorization.
oauth2:
  [ <oauth2> ]

# Whether to enable HTTP2.
[ enable_http2: <bool> | default: true ]

# Optional proxy URL.
[ proxy_url: <string> ]
# Comma-separated string that can contain IPs, CIDR notation, domain names
# that should be excluded from proxying. IP and domain names can
# contain port numbers.
[ no_proxy: <string> ]
# Use proxy URL indicated by environment variables (HTTP_PROXY, http_proxy, HTTPS_PROXY, https_proxy, NO_PROXY, and no_proxy)
[ proxy_from_environment: <boolean> | default: false ]
# Specifies headers to send to proxies during CONNECT requests.
[ proxy_connect_header:
  [ <string>: [<secret>, ...] ] ]

# Configure whether HTTP requests follow HTTP 3xx redirects.
[ follow_redirects: <bool> | default = true ]

# Configures the TLS settings.
tls_config:
  [ <tls_config> ]
```

#### `<oauth2>`

OAuth 2.0 authentication using the client credentials grant type.
Alertmanager fetches an access token from the specified endpoint with
the given client access and secret keys.

```yaml
client_id: <string>
[ client_secret: <secret> ]

# Read the client secret from a file.
# It is mutually exclusive with `client_secret`.
[ client_secret_file: <filename> ]

# Scopes for the token request.
scopes:
  [ - <string> ... ]

# The URL to fetch the token from.
token_url: <string>

# Optional parameters to append to the token URL.
endpoint_params:
  [ <string>: <string> ... ]

# Configures the token request's TLS settings.
tls_config:
  [ <tls_config> ]

# Optional proxy URL.
[ proxy_url: <string> ]
# Comma-separated string that can contain IPs, CIDR notation, domain names
# that should be excluded from proxying. IP and domain names can
# contain port numbers.
[ no_proxy: <string> ]
# Use proxy URL indicated by environment variables (HTTP_PROXY, https_proxy, HTTPs_PROXY, https_proxy, and no_proxy)
[ proxy_from_environment: <boolean> | default: false ]
# Specifies headers to send to proxies during CONNECT requests.
[ proxy_connect_header:
  [ <string>: [<secret>, ...] ] ]
```

#### `<tls_config>`

A `tls_config` allows configuring TLS connections.

```yaml
# CA certificate to validate the server certificate with.
[ ca_file: <filepath> ]

# Certificate and key files for client cert authentication to the server.
[ cert_file: <filepath> ]
[ key_file: <filepath> ]

# ServerName extension to indicate the name of the server.
# http://tools.ietf.org/html/rfc4366#section-3.1
[ server_name: <string> ]

# Disable validation of the server certificate.
[ insecure_skip_verify: <boolean> | default = false]

# Minimum acceptable TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS
# 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
# If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
# See MinVersion in https://pkg.go.dev/crypto/tls#Config.
[ min_version: <string> ]
# Maximum acceptable TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS
# 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
# If unset, Prometheus will use Go default maximum version, which is TLS 1.3.
# See MaxVersion in https://pkg.go.dev/crypto/tls#Config.
[ max_version: <string> ]
```

## Receiver integration settings

These settings allow configuring specific receiver integrations.

### `<discord_config>`

Discord notifications are sent via the [Discord webhook API](https://discord.com/developers/docs/resources/webhook). See Discord's ["Intro to Webhooks" article](https://support.discord.com/hc/en-us/articles/228383668-Intro-to-Webhooks) to learn how to configure a webhook integration for a channel.

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = true ]

# The Discord webhook URL.
# webhook_url and webhook_url_file are mutually exclusive.
webhook_url: <secret>
webhook_url_file: <filepath>

# Message title template.
[ title: <tmpl_string> | default = '{{ template "discord.default.title" . }}' ]

# Message body template.
[ message: <tmpl_string> | default = '{{ template "discord.default.message" . }}' ]

# Message content template. Limited to 2000 characters.
[ content: <tmpl_string> | default = '{{ template "discord.default.content" . }}' ]

# Message username.
[ username: <string> | default = '' ]

# Message avatar URL.
[ avatar_url: <string> | default = '' ]

# The HTTP client's configuration.
[ http_config: <http_config> | default = global.http_config ]
```

### `<email_config>`

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = false ]

# The email address to send notifications to.
# Allows a comma separated list of rfc5322 compliant email addresses.
to: <tmpl_string>

# The sender's address.
[ from: <tmpl_string> | default = global.smtp_from ]

# The SMTP host through which emails are sent.
[ smarthost: <string> | default = global.smtp_smarthost ]

# The hostname to identify to the SMTP server.
[ hello: <string> | default = global.smtp_hello ]

# SMTP authentication information.
# auth_password and auth_password_file are mutually exclusive.
[ auth_username: <string> | default = global.smtp_auth_username ]
[ auth_password: <secret> | default = global.smtp_auth_password ]
[ auth_password_file: <string> | default = global.smtp_auth_password_file ]
[ auth_secret: <secret> | default = global.smtp_auth_secret ]
[ auth_identity: <string> | default = global.smtp_auth_identity ]

# The SMTP TLS requirement.
# Note that Go does not support unencrypted connections to remote SMTP endpoints.
[ require_tls: <bool> | default = global.smtp_require_tls ]

# TLS configuration.
tls_config:
  [ <tls_config> | default = global.smtp_tls_config ]

# The HTML body of the email notification.
[ html: <tmpl_string> | default = '{{ template "email.default.html" . }}' ]
# The text body of the email notification.
[ text: <tmpl_string> ]

# Further headers email header key/value pairs. Overrides any headers
# previously set by the notification implementation.
[ headers: { <string>: <tmpl_string>, ... } ]
```

### `<msteams_config>`

Microsoft Teams notifications are sent via the [Incoming Webhooks](https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/what-are-webhooks-and-connectors) API endpoint.

DEPRECATION NOTICE: Microsoft is deprecating the creation and usage of [Microsoft 365 connectors via Microsoft Teams](https://devblogs.microsoft.com/microsoft365dev/retirement-of-office-365-connectors-within-microsoft-teams/). Consider migrating to using [Workflows](https://learn.microsoft.com/en-us/power-automate/teams/send-a-message-in-teams) with the msteamsv2 config.

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = true ]

# The incoming webhook URL.
# webhook_url and webhook_url_file are mutually exclusive.
[ webhook_url: <secret> ]
[ webhook_url_file: <filepath> ]

# Message title template.
[ title: <tmpl_string> | default = '{{ template "msteams.default.title" . }}' ]

# Message summary template.
[ summary: <tmpl_string> | default = '{{ template "msteams.default.summary" . }}' ]

# Message body template.
[ text: <tmpl_string> | default = '{{ template "msteams.default.text" . }}' ]

# The HTTP client's configuration.
[ http_config: <http_config> | default = global.http_config ]
```

### `<msteamsv2_config>`

Microsoft Teams v2 notifications using the new message format with adaptive cards as required by [flows](https://learn.microsoft.com/en-us/power-automate/teams/overview). Please follow [the documentation](https://support.microsoft.com/en-gb/office/create-incoming-webhooks-with-workflows-for-microsoft-teams-8ae491c7-0394-4861-ba59-055e33f75498) for more information on how to set up this integration.

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = true ]

# The incoming webhook URL.
# webhook_url and webhook_url_file are mutually exclusive.
[ webhook_url: <secret> ]
[ webhook_url_file: <filepath> ]

# Message title template.
[ title: <tmpl_string> | default = '{{ template "msteamsv2.default.title" . }}' ]

# Message body template.
[ text: <tmpl_string> | default = '{{ template "msteamsv2.default.text" . }}' ]

# The HTTP client's configuration.
[ http_config: <http_config> | default = global.http_config ]
```

### `<jira_config>`

JIRA notifications are sent via [JIRA Rest API v2](https://developer.atlassian.com/cloud/jira/platform/rest/v2/intro/)
or [JIRA REST API v3](https://developer.atlassian.com/cloud/jira/platform/rest/v3/intro/#version).

Note: This integration is only tested against a Jira Cloud instance.
Jira Data Center (on premise instance) can work, but it's not guaranteed.

Both APIs have the same feature set. The difference is that V2 supports [Wiki Markup](https://jira.atlassian.com/secure/WikiRendererHelpAction.jspa?section=all)
for the issue description and V3 supports [Atlassian Document Format (ADF)](https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/).
The default `jira.default.description` template only works with V2.

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = true ]

# The URL to send API requests to. The full API path must be included.
# Example: https://company.atlassian.net/rest/api/2/
[ api_url: <string> | default = global.jira_api_url ]

# The project key where issues are created.
project: <string>

# Issue summary template.
[ summary: <tmpl_string> | default = '{{ template "jira.default.summary" . }}' ]

# Issue description template.
[ description: <tmpl_string> | default = '{{ template "jira.default.description" . }}' ]

# Labels to be added to the issue.
labels:
  [ - <tmpl_string> ... ]

# Priority of the issue.
[ priority: <tmpl_string> | default = '{{ template "jira.default.priority" . }}' ]

# Type of the issue (e.g. Bug).
[ issue_type: <string> ]

# Name of the workflow transition to resolve an issue. The target status must have the category "done".
# NOTE: The name of the transition can be localized and depends on the language setting of the service account.
[ resolve_transition: <string> ]

# Name of the workflow transition to reopen an issue. The target status should not have the category "done".
# NOTE: The name of the transition can be localized and depends on the language setting of the service account.
[ reopen_transition: <string> ]

# If reopen_transition is defined, ignore issues with that resolution.
[ wont_fix_resolution: <string> ]

# If reopen_transition is defined, reopen the issue when it is not older than this value (rounded down to the nearest minute).
# The resolutiondate field is used to determine the age of the issue.
[ reopen_duration: <duration> ]

# Other issue and custom fields.
fields:
  [ <string>: <jira_field> ... ]


# The HTTP client's configuration. You must use this configuration to supply the personal access token (PAT) as part of the HTTP `Authorization` header.
# For Jira Cloud, use basic_auth with the email address as the username and the PAT as the password.
# For Jira Data Center, use the 'authorization' field with 'credentials: <PAT value>'.
[ http_config: <http_config> | default = global.http_config ]
```

The `labels` field is a list of labels added to the issue. Template expressions are supported. For example:

```yaml
labels:
  - 'alertmanager'
  - '{{ .CommonLabels.severity }}'
```

#### `<jira_field>`

Jira issue field can have multiple types.
Depends on the field type, the values must be provided differently.
See https://developer.atlassian.com/server/jira/platform/jira-rest-api-examples/#setting-custom-field-data-for-other-field-types for further examples.

```yaml
fields:
    # Components
    components: { name: "Monitoring" }
    # Custom Field TextField
    customfield_10001: "Random text"
    # Custom Field SelectList
    customfield_10002: {"value": "red"}
    # Custom Field MultiSelect
    customfield_10003: [{"value": "red"}, {"value": "blue"}, {"value": "green"}]
```

### `<opsgenie_config>`

OpsGenie notifications are sent via the [OpsGenie API](https://docs.opsgenie.com/docs/alert-api).

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = true ]

# The API key to use when talking to the OpsGenie API.
[ api_key: <secret> | default = global.opsgenie_api_key ]

# The filepath to API key to use when talking to the OpsGenie API. Conflicts with api_key.
[ api_key_file: <filepath> | default = global.opsgenie_api_key_file ]

# The host to send OpsGenie API requests to.
[ api_url: <string> | default = global.opsgenie_api_url ]

# Alert text limited to 130 characters.
[ message: <tmpl_string> | default = '{{ template "opsgenie.default.message" . }}' ]

# A description of the alert.
[ description: <tmpl_string> | default = '{{ template "opsgenie.default.description" . }}' ]

# A backlink to the sender of the notification.
[ source: <tmpl_string> | default = '{{ template "opsgenie.default.source" . }}' ]

# A set of arbitrary key/value pairs that provide further detail
# about the alert.
# All common labels are included as details by default.
[ details: { <string>: <tmpl_string>, ... } ]

# List of responders responsible for notifications.
responders:
  [ - <responder> ... ]

# Comma separated list of tags attached to the notifications.
[ tags: <tmpl_string> ]

# Additional alert note.
[ note: <tmpl_string> ]

# Priority level of alert. Possible values are P1, P2, P3, P4, and P5.
[ priority: <tmpl_string> ]

# Whether to update message and description of the alert in OpsGenie if it already exists
# By default, the alert is never updated in OpsGenie, the new message only appears in activity log.
[ update_alerts: <boolean> | default = false ]

# Optional field that can be used to specify which domain alert is related to.
[ entity: <tmpl_string> ]

# Comma separated list of actions that will be available for the alert.
[ actions: <tmpl_string> ]

# The HTTP client's configuration.
[ http_config: <http_config> | default = global.http_config ]
```

#### `<responder>`

```yaml
# Exactly one of these fields should be defined.
[ id: <tmpl_string> ]
[ name: <tmpl_string> ]
[ username: <tmpl_string> ]

# One of `team`, `teams`, `user`, `escalation` or `schedule`.
#
# The `teams` responder is configured using the `name` field above.
# This field can contain a comma-separated list of team names.
# If the list is empty, no responders are configured.
type: <tmpl_string>
```

### `<pagerduty_config>`

PagerDuty notifications are sent via the [PagerDuty API](https://developer.pagerduty.com/documentation/integration/events).
PagerDuty provides [documentation](https://www.pagerduty.com/docs/guides/prometheus-integration-guide/) on how to integrate. There are important differences with Alertmanager's v0.11 and greater support of PagerDuty's Events API v2.

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = true ]

# The routing and service keys are mutually exclusive.
# The PagerDuty integration key (when using PagerDuty integration type `Events API v2`).
# It is mutually exclusive with `routing_key_file`.
routing_key: <tmpl_secret>
# Read the Pager Duty routing key from a file.
# It is mutually exclusive with `routing_key`.
routing_key_file: <filepath>
# The PagerDuty integration key (when using PagerDuty integration type `Prometheus`).
# It is mutually exclusive with `service_key_file`.
service_key: <tmpl_secret>
# Read the Pager Duty service key from a file.
# It is mutually exclusive with `service_key`.
service_key_file: <filepath>

# The URL to send API requests to
[ url: <string> | default = global.pagerduty_url ]

# The client identification of the Alertmanager.
[ client:  <tmpl_string> | default = '{{ template "pagerduty.default.client" . }}' ]
# A backlink to the sender of the notification.
[ client_url:  <tmpl_string> | default = '{{ template "pagerduty.default.clientURL" . }}' ]

# A description of the incident.
[ description: <tmpl_string> | default = '{{ template "pagerduty.default.description" .}}' ]

# Severity of the incident.
[ severity: <tmpl_string> | default = 'error' ]

# Unique location of the affected system.
[ source: <tmpl_string> | default = client ]

# A set of arbitrary key/value pairs that provide further detail
# about the incident.
[ details: { <string>: <tmpl_string>, ... } | default = {
  firing:       '{{ template "pagerduty.default.instances" .Alerts.Firing }}'
  resolved:     '{{ template "pagerduty.default.instances" .Alerts.Resolved }}'
  num_firing:   '{{ .Alerts.Firing | len }}'
  num_resolved: '{{ .Alerts.Resolved | len }}'
} ]

# Images to attach to the incident.
images:
  [ <image_config> ... ]

# Links to attach to the incident.
links:
  [ <link_config> ... ]

# The part or component of the affected system that is broken.
[ component: <tmpl_string> ]

# A cluster or grouping of sources.
[ group: <tmpl_string> ]

# The class/type of the event.
[ class: <tmpl_string> ]

# The HTTP client's configuration.
[ http_config: <http_config> | default = global.http_config ]
```

#### `<image_config>`

The fields are documented in the [PagerDuty API documentation](https://developer.pagerduty.com/docs/events-api-v2/trigger-events/#the-images-property).

```yaml
href: <tmpl_string>
src: <tmpl_string>
alt: <tmpl_string>
```

#### `<link_config>`

The fields are documented in the [PagerDuty API documentation](https://developer.pagerduty.com/docs/events-api-v2/trigger-events/#the-links-property).

```yaml
href: <tmpl_string>
text: <tmpl_string>
```

### `<pushover_config>`

Pushover notifications are sent via the [Pushover API](https://pushover.net/api).

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = true ]

# The recipient user's key.
# user_key and user_key_file are mutually exclusive.
user_key: <secret>
user_key_file: <filepath>

# Your registered application's API token, see https://pushover.net/apps
# You can also register a token by cloning this Prometheus app:
# https://pushover.net/apps/clone/prometheus
# token and token_file are mutually exclusive.
token: <secret>
token_file: <filepath>

# Notification title.
[ title: <tmpl_string> | default = '{{ template "pushover.default.title" . }}' ]

# Notification message.
[ message: <tmpl_string> | default = '{{ template "pushover.default.message" . }}' ]

# A supplementary URL shown alongside the message.
[ url: <tmpl_string> | default = '{{ template "pushover.default.url" . }}' ]

# Optional device to send notification to, see https://pushover.net/api#device
[ device: <string> ]

# Optional sound to use for notification, see https://pushover.net/api#sound
[ sound: <string> ]

# Priority, see https://pushover.net/api#priority
[ priority: <tmpl_string> | default = '{{ if eq .Status "firing" }}2{{ else }}0{{ end }}' ]

# How often the Pushover servers will send the same notification to the user.
# Must be at least 30 seconds.
[ retry: <duration> | default = 1m ]

# How long your notification will continue to be retried for, unless the user
# acknowledges the notification.
[ expire: <duration> | default = 1h ]

# Optional time to live (TTL) to use for notification, see https://pushover.net/api#ttl
[ ttl: <duration> ]

# Optional HTML/monospace formatting for the message, see https://pushover.net/api#html
# html and monospace formatting are mutually exclusive.
[ html: <boolean> | default = false ]
[ monospace: <boolean> | default = false ]

# The HTTP client's configuration.
[ http_config: <http_config> | default = global.http_config ]
```

### `<rocketchat_config>`

Rocketchat notifications are sent via the [Rocketchat REST API](https://developer.rocket.chat/reference/api/rest-api/endpoints/messaging/chat-endpoints/postmessage).

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = true ]
[ api_url: <string> | default = global.rocketchat_api_url ]
[ channel: <tmpl_string> | default = global.rocketchat_api_url ]

# The sender token and token_id
# See https://docs.rocket.chat/use-rocket.chat/user-guides/user-panel/my-account#personal-access-tokens
# token and token_file are mutually exclusive.
# token_id and token_id_file are mutually exclusive.
token: <secret>
token_file: <filepath>
token_id: <secret>
token_id_file: <filepath>


[ color: <tmpl_string | default '{{ if eq .Status "firing" }}red{{ else }}green{{ end }}' ]
[ emoji <tmpl_string | default = '{{ template "rocketchat.default.emoji" . }}'
[ icon_url <tmpl_string | default = '{{ template "rocketchat.default.iconurl" . }}'
[ text <tmpl_string | default = '{{ template "rocketchat.default.text" . }}'
[ title <tmpl_string | default = '{{ template "rocketchat.default.title" . }}'
[ titleLink <tmpl_string | default = '{{ template "rocketchat.default.titlelink" . }}'
[ text: <tmpl_string | default = '{{ template "rocketchat.default.text" . }}'
fields:
  [ <rocketchat_field_config> ... ]
[ image_url <tmpl_string> ]
[ thumb_url <tmpl_string> ]
[ link_names <tmpl_string> ]
[ short_fields: <boolean> | default = false ]
actions:
  [ <rocketchat_action_config> ... ]
```

#### `<rocketchat_field_config>`

The fields are documented in the [Rocketchat API documentation](https://developer.rocket.chat/reference/api/rest-api/endpoints/messaging/chat-endpoints/postmessage#attachment-field-objects).

```yaml
[ title: <tmpl_string> ]
[ value: <tmpl_string> ]
[ short: <boolean> | default = rocketchat_config.short_fields ]
```

#### `<rocketchat_action_config>`

The fields are documented in the [Rocketchat API api models](https://github.com/RocketChat/Rocket.Chat.Go.SDK/blob/master/models/message.go).

```yaml
[ type: <tmpl_string> | ignored, only "button" is supported ]
[ text: <tmpl_string> ]
[ url: <tmpl_string> ]
[ msg: <tmpl_string> ]
```

### `<slack_config>`

Slack notifications can be sent via [Incoming webhooks](https://api.slack.com/messaging/webhooks) or [Bot tokens](https://api.slack.com/authentication/token-types).

If using an incoming webhook then `api_url` must be set to the URL of the incoming webhook, or written to the file referenced in `api_url_file`.

If using Bot tokens then `api_url` must be set to [`https://slack.com/api/chat.postMessage`](https://api.slack.com/methods/chat.postMessage), the bot token must be set as the authorization credentials in `http_config`, and `channel` must contain either the name of the channel or Channel ID to send notifications to. If using the name of the channel the # is optional.

The notification contains an [attachment](https://api.slack.com/messaging/composing/layouts#attachments).

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = false ]

# The Slack webhook URL. Either api_url or api_url_file should be set.
# Defaults to global settings if none are set here.
[ api_url: <secret> | default = global.slack_api_url ]
[ api_url_file: <filepath> | default = global.slack_api_url_file ]

# The channel or user to send notifications to.
channel: <tmpl_string>

# API request data as defined by the Slack webhook API.
[ icon_emoji: <tmpl_string> ]
[ icon_url: <tmpl_string> ]
[ link_names: <boolean> | default = false ]
[ username: <tmpl_string> | default = '{{ template "slack.default.username" . }}' ]
# The following parameters define the attachment.
actions:
  [ <action_config> ... ]
[ callback_id: <tmpl_string> | default = '{{ template "slack.default.callbackid" . }}' ]
[ color: <tmpl_string> | default = '{{ if eq .Status "firing" }}danger{{ else }}good{{ end }}' ]
[ fallback: <tmpl_string> | default = '{{ template "slack.default.fallback" . }}' ]
fields:
  [ <field_config> ... ]
[ footer: <tmpl_string> | default = '{{ template "slack.default.footer" . }}' ]
[ mrkdwn_in: '[' <string>, ... ']' | default = ["fallback", "pretext", "text"] ]
[ pretext: <tmpl_string> | default = '{{ template "slack.default.pretext" . }}' ]
[ short_fields: <boolean> | default = false ]
[ text: <tmpl_string> | default = '{{ template "slack.default.text" . }}' ]
[ title: <tmpl_string> | default = '{{ template "slack.default.title" . }}' ]
[ title_link: <tmpl_string> | default = '{{ template "slack.default.titlelink" . }}' ]
[ image_url: <tmpl_string> ]
[ thumb_url: <tmpl_string> ]

# The HTTP client's configuration.
[ http_config: <http_config> | default = global.http_config ]
```

#### `<action_config>`

The fields are documented in the Slack API documentation for [message attachments](https://api.slack.com/messaging/composing/layouts#attachments) and [interactive messages](https://api.slack.com/legacy/interactive-message-field-guide#action_fields).

```yaml
text: <tmpl_string>
type: <tmpl_string>
# Either url or name and value are mandatory.
[ url: <tmpl_string> ]
[ name: <tmpl_string> ]
[ value: <tmpl_string> ]

[ confirm: <action_confirm_field_config> ]
[ style: <tmpl_string> | default = '' ]
```

##### `<action_confirm_field_config>`

The fields are documented in the [Slack API documentation](https://api.slack.com/legacy/interactive-message-field-guide#confirmation_fields).

```yaml
text: <tmpl_string>
[ dismiss_text: <tmpl_string> | default '' ]
[ ok_text: <tmpl_string> | default '' ]
[ title: <tmpl_string> | default '' ]
```

#### `<field_config>`

The fields are documented in the [Slack API documentation](https://api.slack.com/messaging/composing/layouts#attachments).

```yaml
title: <tmpl_string>
value: <tmpl_string>
[ short: <boolean> | default = slack_config.short_fields ]
```

### `<sns_config>`

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = true ]

# The SNS API URL i.e. https://sns.us-east-2.amazonaws.com.
#  If not specified, the SNS API URL from the SNS SDK will be used.
[ api_url: <tmpl_string> ]

# Configures AWS's Signature Verification 4 signing process to sign requests.
sigv4:
  [ <sigv4_config> ]

# SNS topic ARN, i.e. arn:aws:sns:us-east-2:698519295917:My-Topic
# If you don't specify this value, you must specify a value for the phone_number or target_arn.
# If you are using a FIFO SNS topic you should set a message group interval longer than 5 minutes
# to prevent messages with the same group key being deduplicated by the SNS default deduplication window
[ topic_arn: <tmpl_string> ]

# Subject line when the message is delivered to email endpoints.
[ subject: <tmpl_string> | default = '{{ template "sns.default.subject" .}}' ]

# Phone number if message is delivered via SMS in E.164 format.
# If you don't specify this value, you must specify a value for the topic_arn or target_arn.
[ phone_number: <tmpl_string> ]

# The  mobile platform endpoint ARN if message is delivered via mobile notifications.
# If you don't specify this value, you must specify a value for the topic_arn or phone_number.
[ target_arn: <tmpl_string> ]

# The message content of the SNS notification.
[ message: <tmpl_string> | default = '{{ template "sns.default.message" .}}' ]

# SNS message attributes.
attributes:
  [ <string>: <string> ... ]

# The HTTP client's configuration.
[ http_config: <http_config> | default = global.http_config ]
```

#### `<sigv4_config>`

```yaml
# The AWS region. If blank, the region from the default credentials chain is used.
[ region: <string> ]

# The AWS API keys. Both access_key and secret_key must be supplied or both must be blank.
# If blank the environment variables `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` are used.
[ access_key: <string> ]
[ secret_key: <secret> ]

# Named AWS profile used to authenticate.
[ profile: <string> ]

# AWS Role ARN, an alternative to using AWS API keys.
[ role_arn: <string> ]
```

### `<telegram_config>`

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = true ]

# The Telegram API URL i.e. https://api.telegram.org.
# If not specified, default API URL will be used.
[ api_url: <string> | default = global.telegram_api_url ]

# Telegram bot token. It is mutually exclusive with `bot_token_file`.
[ bot_token: <secret> ]

# Read the Telegram bot token from a file. It is mutually exclusive with `bot_token`.
[ bot_token_file: <filepath> ]

# ID of the chat where to send the messages.
[ chat_id: <int> ]

# Optional ID of the message thread where to send the messages.
[ message_thread_id: <int> ]

# Message template.
[ message: <tmpl_string> default = '{{ template "telegram.default.message" .}}' ]

# Disable telegram notifications
[ disable_notifications: <boolean> | default = false ]

# Parse mode for telegram message, supported values are MarkdownV2, Markdown, HTML and empty string for plain text.
[ parse_mode: <string> | default = "HTML" ]

# The HTTP client's configuration.
[ http_config: <http_config> | default = global.http_config ]
```

### `<victorops_config>`

VictorOps notifications are sent out via the [VictorOps API](https://help.victorops.com/knowledge-base/rest-endpoint-integration-guide/)

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = true ]

# The API key to use when talking to the VictorOps API.
# It is mutually exclusive with `api_key_file`.
[ api_key: <secret> | default = global.victorops_api_key ]

# Reads the API key to use when talking to the VictorOps API from a file.
# It is mutually exclusive with `api_key`.
[ api_key_file: <filepath> | default = global.victorops_api_key_file ]

# The VictorOps API URL.
[ api_url: <string> | default = global.victorops_api_url ]

# A key used to map the alert to a team.
routing_key: <tmpl_string>

# Describes the behavior of the alert (CRITICAL, WARNING, INFO).
[ message_type: <tmpl_string> | default = 'CRITICAL' ]

# Contains summary of the alerted problem.
[ entity_display_name: <tmpl_string> | default = '{{ template "victorops.default.entity_display_name" . }}' ]

# Contains long explanation of the alerted problem.
[ state_message: <tmpl_string> | default = '{{ template "victorops.default.state_message" . }}' ]

# The monitoring tool the state message is from.
[ monitoring_tool: <tmpl_string> | default = '{{ template "victorops.default.monitoring_tool" . }}' ]

# The HTTP client's configuration.
[ http_config: <http_config> | default = global.http_config ]
```

### `<webhook_config>`

The webhook receiver allows configuring a generic receiver.

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = true ]

# The endpoint to send HTTP POST requests to.
# url and url_file are mutually exclusive.
url: <secret>
url_file: <filepath>

# The HTTP client's configuration.
[ http_config: <http_config> | default = global.http_config ]

# The maximum number of alerts to include in a single webhook message. Alerts
# above this threshold are truncated. When leaving this at its default value of
# 0, all alerts are included.
[ max_alerts: <int> | default = 0 ]

# The maximum time to wait for a webhook request to complete, before failing the
# request and allowing it to be retried. The default value of 0s indicates that
# no timeout should be applied.
# NOTE: This will have no effect if set higher than the group_interval.
[ timeout: <duration> | default = 0s ]
```

The Alertmanager
will send HTTP POST requests in the following JSON format to the configured
endpoint:

```
{
  "version": "4",
  "groupKey": <string>,              // key identifying the group of alerts (e.g. to deduplicate)
  "truncatedAlerts": <int>,          // how many alerts have been truncated due to "max_alerts"
  "status": "<resolved|firing>",
  "receiver": <string>,
  "groupLabels": <object>,
  "commonLabels": <object>,
  "commonAnnotations": <object>,
  "externalURL": <string>,           // backlink to the Alertmanager.
  "alerts": [
    {
      "status": "<resolved|firing>",
      "labels": <object>,
      "annotations": <object>,
      "startsAt": "<rfc3339>",
      "endsAt": "<rfc3339>",
      "generatorURL": <string>,      // identifies the entity that caused the alert
      "fingerprint": <string>        // fingerprint to identify the alert
    },
    ...
  ]
}
```

There is a list of
[integrations](https://prometheus.io/docs/operating/integrations/#alertmanager-webhook-receiver) with
this feature.

### `<wechat_config>`

WeChat notifications are sent via the [WeChat
API](https://developers.weixin.qq.com/doc/offiaccount/en/Message_Management/Service_Center_messages.html).

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = false ]

# The API key to use when talking to the WeChat API.
[ api_secret: <secret> | default = global.wechat_api_secret ]

# The WeChat API URL.
[ api_url: <string> | default = global.wechat_api_url ]

# The corp id for authentication.
[ corp_id: <string> | default = global.wechat_api_corp_id ]

# API request data as defined by the WeChat API.
[ message: <tmpl_string> | default = '{{ template "wechat.default.message" . }}' ]
# Type of the message type, supported values are `text` and `markdown`.
[ message_type: <string> | default = 'text' ]
[ agent_id: <string> | default = '{{ template "wechat.default.agent_id" . }}' ]
[ to_user: <string> | default = '{{ template "wechat.default.to_user" . }}' ]
[ to_party: <string> | default = '{{ template "wechat.default.to_party" . }}' ]
[ to_tag: <string> | default = '{{ template "wechat.default.to_tag" . }}' ]
```

### `<webex_config>`

```yaml
# Whether to notify about resolved alerts.
[ send_resolved: <boolean> | default = true ]

# The Webex Teams API URL i.e. https://webexapis.com/v1/messages
# If not specified, default API URL will be used.
[ api_url: <string> | default = global.webex_api_url ]

# ID of the Webex Teams room where to send the messages.
room_id: <tmpl_string>

# Message template.
[ message: <tmpl_string> default = '{{ template "webex.default.message" .}}' ]

# The HTTP client's configuration. You must use this configuration to supply the bot token as part of the HTTP `Authorization` header.
[ http_config: <http_config> | default = global.http_config ]
```
