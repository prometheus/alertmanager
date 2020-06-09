---
title: Alerting overview
sort_rank: 1
nav_icon: sliders
---

# Alerting Overview

Alerting with Prometheus is separated into two parts. Alerting rules in
Prometheus servers send alerts to an Alertmanager. The [Alertmanager](../alertmanager)
then manages those alerts, including silencing, inhibition, aggregation and
sending out notifications via methods such as email, on-call notification systems, and chat platforms.

The main steps to setting up alerting and notifications are:

* Setup and [configure](../configuration) the Alertmanager
* [Configure Prometheus](../../prometheus/latest/configuration/configuration/#alertmanager_config) to talk to the Alertmanager
* Create [alerting rules](../../prometheus/latest/configuration/alerting_rules/) in Prometheus
