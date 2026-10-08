---
domain: routes
availability:
  - { place: amtool }
references:
  - kind: code
    role: implementation
    target: cli/test_routing.go
  - kind: code
    role: implementation
    target: dispatch/route.go#Route.Match
---

# Test routes

An operator checks which receivers an alert with a given set of labels would
reach, matched the same way live alerts are, and can assert the expected
receivers in a script.
