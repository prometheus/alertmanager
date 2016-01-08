# Alertmanager

This is the development version of the Alertmanager. It is a rewrite and
is incompatible to the present version 0.0.4. The only backport was the API endpoint used by Prometheus to push new alerts.

## Installation

### Dependencies

Debian family:

    sudo apt-get install build-essential libc6-dev

Red Hat family:

    sudo yum install glibc-static

### Compiling the binary

The current version has to be run from the repository folder as UI assets and notification templates are not yet statically compiled into the binary.

You can either `go get` it:

```
$ GO15VENDOREXPERIMENT=1 go get github.com/prometheus/alertmanager
# cd $GOPATH/src/github.com/prometheus/alertmanager
$ alertmanager -config.file=<your_file>
```

Or checkout the source code and build manually:

```
$ mkdir -p $GOPATH/src/github.com/prometheus
$ cd $GOPATH/src/github.com/prometheus
$ git clone https://github.com/prometheus/alertmanager.git
$ cd alertmanager
$ make
$ ./alertmanager -config.file=<your_file>
```

## Status

This version was written from scratch. Core features enabled by this is are more advanced alert routing configurations and grouping/batching of alerts. Thus, squashing expression results through aggregation in alerting rules is no longer required to avoid noisyness.

The concepts of alert routing were outlined in [this document](https://docs.google.com/document/d/1-4jefGkFo71jlaLo4lHz40ZBoCv9ycBBBbjzbXifGyY/edit?usp=sharing).

The version implements full persistence of alerts, silences, and notification state. On restart it picks up right where it left off.

### Known issues

This development version still has an extensive list of improvements and changes. This is an incomplete list of things that are still missing or need to be improved.

This will happen based on priority and demand. Feel free to ping fabxc about it

* On deleting silences it may take up to one `group_wait` cycle for a notification of a previously silenced alert to be sent.
* Limiting inhibition rules to routing subtrees to avoid accidental interference
* Definition of a minimum data set provided to notification templates
* Best practices around notification templating
* Various common command line flags like `path-prefix`
* Compiling templates and UI assets into the binary

## Example

This is an example configuration that should cover most relevant aspects of the new YAML configuration format. Authoritative source for now is the [code](https://github.com/prometheus/alertmanager/tree/dev/config).

```yaml
global:
  # The smarthost and SMTP sender used for mail notifications.
  smtp_smarthost: 'localhost:25'
  smtp_from: 'alertmanager@example.org'

# The root route on which each incoming alert enters.
route:
  # The labels by which incoming alerts are grouped together. For example,
  # multiple alerts coming in for cluster=A and alertname=LatencyHigh would
  # be batched into a single group.
  group_by: ['alertname', 'cluster']
  
  # When a new group of alerts is created by an incoming alert, wait at
  # least 'group_wait' to send the initial notification.
  # This way ensures that you get multiple alerts for the same group that start
  # firing shortly after another are batched together on the first 
  # notification.
  group_wait: 30s

  # When the first notification was sent, wait 'group_interval' to send a betch
  # of new alerts that started firing for that group.
  group_interval: 5m

  # If an alert has successfully been sent, wait 'repeat_interval' to
  # resend them.
  repeat_interval: 3h 

  # If 'continue' is false, the first sub-route that matches this alert will
  # terminate the search and the alert will be inserted at that routing node.
  # If true, the alert is inserted to sibling nodes as well if there is a
  # match.
  # This allows to do first-match semantics (=false) in smaller scopes (e.g. team-level),
  # while avoiding accidental shadowing (=true) at alerts at larger scopes (e.g. company-level)
  continue: true

  # All the above attributes are inherited by all child routes and can 
  # overwritten on each.

  # The child route trees.
  routes:
  # This routes performs a regular expression match on alert labels to
  # catch alerts that are related to a list of services.
  - match_re:
      service: ^(foo1|foo2|baz)$
    receiver: team-X-mails

    # The service has a sub-route for critical alerts, any alerts
    # that do not match, i.e. severity != critical, fall-back to the
    # parent node and are sent to 'team-X-mails'
    routes:
    - match:
        severity: critical
      receiver: team-X-pager

  - match:
      service: files
    receiver: team-Y-mails

    routes:
    - match:
        severity: critical
      receiver: team-Y-pager

  # This route handles all alerts coming from a database service. If there's
  # no team to handle it, it defaults to the DB team.
  - match:
      service: database

    receiver: team-DB-pager
    # Also group alerts by affected database.
    group_by: [alertname, cluster, database]
    continue: false

    routes:
    - match:
        owner: team-X
      receiver: team-X-pager

    - match:
        owner: team-Y
      receiver: team-Y-pager


# Inhibition rules allow to mute a set of alerts given that another alert is
# firing.
# We use this to mute any warning-level notifications if the same alert is 
# already critical.
inhibit_rules:
- source_match:
    severity: 'critical'
  target_match:
    severity: 'warning'
  # Apply inhibition if the alertname is the same.
  equal: ['alertname']


receivers:
- name: 'team-X-mails'
  email_configs:
  - to: 'team-X+alerts@example.org'

- name: 'team-X-pager'
  email_configs:
  - to: 'team-X+alerts-critical@example.org'
  pagerduty_configs:
  - service_key: <team-X-key>

- name: 'team-Y-mails'
  email_configs:
  - to: 'team-Y+alerts@example.org'

- name: 'team-Y-pager'
  pagerduty_configs:
  - service_key: <team-Y-key>

- name: 'team-DB-pager'
  pagerduty_configs:
  - service_key: <team-DB-key>
```

## Testing

If you want to test the new Alertmanager while running the current version, you can mirror traffic to the new one with a simple nginx configuration similar to this:

```
server {
  server_name <your_current_alertmanager>;
  location / {
    proxy_pass  http://localhost:9093;
    post_action @forward;
  }
  location @forward {
    proxy_pass http://<your_new_alertmanager>:9093; 
  }
}
```

## Architecture

![](https://raw.githubusercontent.com/prometheus/alertmanager/4e6695682acd2580773a904e4aa2e3b927ee27b7/doc/arch.jpg)

