---
appliesTo:
  - { type: capability, id: route-alerts }
  - { type: capability, id: test-routes }
  - { type: entity, id: route }
references:
  - kind: code
    role: implementation
    target: dispatch/route.go#Route.Match
---

# Routes are matched depth first in the order they are written

An alert goes down into the first child route it matches and stops there,
unless that route says to continue, in which case it is also matched against
the later siblings. An alert that matches a route but none of its children
stays with that route.
