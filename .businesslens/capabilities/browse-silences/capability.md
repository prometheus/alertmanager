---
domain: silences
availability:
  - { place: web-ui }
  - { place: api }
  - { place: amtool }
references:
  - kind: code
    role: implementation
    target: api/v2/api.go#getSilencesHandler
  - kind: code
    role: implementation
    target: api/v2/api.go#getSilenceHandler
  - kind: code
    role: implementation
    target: cli/silence_query.go
  - kind: code
    role: implementation
    target: ui/app/src/Views/SilenceView/Updates.elm
---

# Browse silences

Operators list silences by state, narrowed by label matchers, and open one silence
to see everything it keeps and, in the web UI, the alerts it currently
affects.
