---
title: Alerting overview
sort_rank: 1
nav_icon: sliders
---

# Alerting Overview

Alerting with Prometheus is separated into two parts. Alerting rules in
Prometheus servers send alerts to an Alertmanager. The [Alertmanager](alertmanager.md)
then manages those alerts, including silencing, inhibition, aggregation and
sending out notifications via methods such as email, PagerDuty and HipChat.

The main steps to setting up alerting and notifications are:

* Setup and [configure](configuration.md) the Alertmanager
* Configure Prometheus to talk to the Alertmanager with the `-alertmanager.url` flag
* Create [alerting rules](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules) in Prometheus
