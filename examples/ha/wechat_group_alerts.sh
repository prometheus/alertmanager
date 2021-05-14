#!/usr/bin/env bash
alerts1='[
    {
        "labels": {
            "application": "app1",
            "instance": "instance1",
            "groupTitle": "微服务测试"
        },
        "startsAt": "2021-01-20T14:31:39.000Z",
        "endsAt": "2021-06-28T04:09:43.534Z",
        "annotations": {
            "summary": "summary info",
            "message": "message content"
        },
        "generatorURL": "http://GecyXAtxISsqBCNScCD.xldw-xtmZQKncZJequKpc,2CQ1J7lE-SKEx2i1dYxWDbo071rZFoiN7OQEbKltIARN+"
    },
    {
        "labels": {
            "application": "app1",
            "instance": "instance1",
            "groupTitle": "微服务测试"
        },
        "startsAt": "2021-01-20T14:31:39.000Z",
        "endsAt": "2021-06-28T04:09:43.534Z",
        "annotations": {
            "summary": "summary info2",
            "message": "message content2"
        },
        "generatorURL": "http://GecyXAtxISsqBCNScCD.xldw-xtmZQKncZJequKpc,2CQ1J7lE-SKEx2i1dYxWDbo071rZFoiN7OQEbKltIARN+"
    }
]'
curl -XPOST -d"$alerts1" -H "Content-Type:application/json" http://localhost:9093/api/v2/alerts
