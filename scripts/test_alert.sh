#!/bin/bash
set -euo pipefail

curl -H 'Content-Type: application/json' -d '[{"labels":{"alertname":"Test Alert", "severity": "warning", "env": "test", "host_name": "testmachine"}}]' http://127.0.0.1:9093/api/v2/alerts
