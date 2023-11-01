#!/bin/bash

alertname=${1:-$RANDOM}
url='http://localhost:9093/api/v1/alerts'
echo "Firing up alert"
curl -XPOST $url -d '[{"status": "firing","labels": {"alertname": "'$alertname'","service": "curl","severity": "warning","instance": "0"},"annotations": {"summary": "This is a summary","description": "This is a description."},"generatorURL": "http://prometheus.int.example.net/<generating_expression>","startsAt": "2020-07-23T01:05:36+00:00"}]'
echo ""

echo "press enter to resolve alert"
read

echo "sending resolve"
curl -XPOST $url -d '[{"status": "resolved","labels": {"alertname": "'$alertname'","service": "curl","severity": "warning","instance": "0"},"annotations": {"summary": "This is a summary","description": "This is a description."},"generatorURL": "http://prometheus.int.example.net/<generating_expression>","startsAt": "2020-07-23T01:05:36+00:00","endsAt": "2020-07-23T01:05:38+00:00"}]'
echo ""
