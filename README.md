# Alertmanager

This is the development version of the Alertmanager. It is a rewrite and
is only compatible to the present version 0.0.4 in terms of the API endpoint
used by Prometheus to push new alerts.

## Installation

You can either `go get` it:

```
$ GO15VENDOREXPERIMENT=1 go get github.com/prometheus/alertmanager
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

The version implements full persistence of alerts, silences, and notification state. On restart it picks up right where it left off.

### Known issues

This development version still has an extensive list of improvements and changes. This is an incomplete list of things that are still missing or need to be improved.

* On deleting silences it may take up to one `group_wait` cycle for a notification of a previously silenced alert to be sent.
* Limiting inhibition rules to routing subtrees to avoid accidental interference.
* Show silencing inhibition of alerts in the UI
* Better overview for alert groups in UI
* Status page in UI
* `Silence` button next to alerts in UI being usable
* Definition of a minimum data set provided to notification templates
* Best practices around notification templating
* Various common command line flags like `path-prefix`



