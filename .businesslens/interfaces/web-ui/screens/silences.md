---
entities:
  - entity: silence
    shows: [Matchers, Starts at, Ends at, Annotations]
entryPoints:
  - web-ui: /#/silences
references:
  - kind: code
    role: implementation
    target: ui/app/src/Views/SilenceList/Views.elm
  - kind: code
    role: implementation
    target: ui/app/src/Views/SilenceList/SilenceView.elm
---

# Silences

Silences separated into active, pending and expired ones with a count for
each, narrowed by label matchers.
