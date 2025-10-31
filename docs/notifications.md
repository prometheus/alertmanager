---
title: Notification template reference
sort_rank: 7
---

Prometheus creates and sends alerts to the Alertmanager which then sends notifications out to different receivers based on their labels.
A receiver can be one of many integrations including: Slack, PagerDuty, email, or a custom integration via the generic webhook interface.

The notifications sent to receivers are constructed via templates. The Alertmanager comes with default templates but they can also be customized.
To avoid confusion it's important to note that the Alertmanager templates differ from [templating in Prometheus](https://prometheus.io/docs/visualization/template_reference/), however Prometheus templating also includes the templating in alert rule labels/annotations.


The Alertmanager's notification templates are based on the [Go templating](http://golang.org/pkg/text/template) system.
Note that some fields are evaluated as text, and others as HTML which will affect escaping.

# Data Structures

## Data

`Data` is the structure passed to notification templates and webhook pushes.

| Name          | Type     | Notes    |
| ------------- | ------------- | -------- |
| Receiver | string | Defines the receiver's name that the notification will be sent to (slack, email etc.). |
| Status | string | Defined as firing if at least one alert is firing, otherwise resolved. |
| Alerts | [Alert](#alert) | List of all alert objects in this group ([see below](#alert)). |
| GroupLabels | [KV](#kv) | The labels these alerts were grouped by. |
| CommonLabels | [KV](#kv) | The labels common to all of the alerts. |
| CommonAnnotations | [KV](#kv) | Set of common annotations to all of the alerts. Used for longer additional strings of information about the alert. |
| ExternalURL | string | Backlink to the Alertmanager that sent the notification. |

The `Alerts` type exposes functions for filtering alerts:

 - `Alerts.Firing` returns a list of currently firing alert objects in this group
 - `Alerts.Resolved` returns a list of resolved alert objects in this group

## Alert

`Alert` holds one alert for notification templates.

| Name          | Type     | Notes    |
| ------------- | ------------- | -------- |
| Status | string | Defines whether or not the alert is resolved or currently firing. |
| Labels | [KV](#kv) | A set of labels to be attached to the alert. |
| Annotations | [KV](#kv) | A set of annotations for the alert. |
| StartsAt | time.Time | The time the alert started firing. If omitted, the current time is assigned by the Alertmanager. |
| EndsAt | time.Time | Only set if the end time of an alert is known. Otherwise set to a configurable timeout period from the time since the last alert was received. |
| GeneratorURL | string | A backlink which identifies the causing entity of this alert. |
| Fingerprint | string | Fingerprint that can be used to identify the alert. |

## KV

`KV` is a set of key/value string pairs used to represent labels and annotations.

```
type KV map[string]string
```

Annotation example containing two annotations:

```
{
  summary: "alert summary",
  description: "alert description",
}
```

In addition to direct access of data (labels and annotations) stored as KV, there are also methods for sorting, removing, and viewing the LabelSets:

### KV methods
| Name          | Arguments     | Returns  | Notes    |
| ------------- | ------------- | -------- | -------- |
| SortedPairs | - | Pairs (list of key/value string pairs.) | Returns a sorted list of key/value pairs. |
| Remove | []string | KV | Returns a copy of the key/value map without the given keys. |
| Names | - | []string | Returns the names of the label names in the LabelSet. |
| Values | - | []string | Returns a list of the values in the LabelSet. |

# Functions

Note the [default
functions](http://golang.org/pkg/text/template/#hdr-Functions) also provided by Go
templating.

## Strings

| Name          | Arguments     | Returns  | Notes    |
| ------------- | ------------- | -------- | -------- |
| title | string |[strings.Title](http://golang.org/pkg/strings/#Title), capitalises first character of each word. |
| toUpper | string | [strings.ToUpper](http://golang.org/pkg/strings/#ToUpper), converts all characters to upper case. |
| toLower | string | [strings.ToLower](http://golang.org/pkg/strings/#ToLower), converts all characters to lower case. |
| trimSpace | string | [strings.TrimSpace](https://pkg.go.dev/strings#TrimSpace), removes leading and trailing white spaces. |
| match | pattern, string | [Regexp.MatchString](https://golang.org/pkg/regexp/#MatchString). Match a string using Regexp. |
| reReplaceAll | pattern, replacement, text | [Regexp.ReplaceAllString](http://golang.org/pkg/regexp/#Regexp.ReplaceAllString) Regexp substitution, unanchored. |
| join | sep string, s []string | [strings.Join](http://golang.org/pkg/strings/#Join), concatenates the elements of s to create a single string. The separator string sep is placed between elements in the resulting string. (note: argument order inverted for easier pipelining in templates.) |
| safeHtml | text string | [html/template.HTML](https://golang.org/pkg/html/template/#HTML), Marks string as HTML not requiring auto-escaping. |
| stringSlice | ...string | Returns the passed strings as a slice of strings. |
| date | string, time.Time | Returns the text representation of the time in the specified format. For documentation on formats refer to [pkg.go.dev/time](https://pkg.go.dev/time#pkg-constants). |
| tz | string, time.Time | Returns the time in the timezone. For example, Europe/Paris. |
| since | time.Time | [time.Duration](https://pkg.go.dev/time#Since), returns the duration of how much time passed from the provided time till the current system time. |
| humanizeDuration | number or string | Returns a human-readable string representing the duration, and the error if it happened. |
