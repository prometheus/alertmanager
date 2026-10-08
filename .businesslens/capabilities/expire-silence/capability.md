---
domain: silences
availability:
  - { place: web-ui }
  - { place: api }
  - { place: amtool }
references:
  - kind: code
    role: implementation
    target: silence/silence.go#Silences.Expire
  - kind: code
    role: implementation
    target: api/v2/api.go#deleteSilenceHandler
  - kind: code
    role: implementation
    target: cli/silence_expire.go
  - kind: code
    role: implementation
    target: ui/app/src/Views/SilenceList/Updates.elm
---

# Expire silence

An operator ends a silence before its end time. An active silence stops muting at
once; a pending one, shown with a Delete control, never starts. Alertmanager
also expires every silence on its own when its end time passes.
