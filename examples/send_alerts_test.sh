alerts1='[
  {
    "labels": {
       "alertname": "DiskRunningFull",
       "dev": "sda1",
       "instance": "example3"
       "severity": "high"
     }
  }
]'
curl -XPOST -d"$alerts1" http://localhost:9093/api/v1/alerts
