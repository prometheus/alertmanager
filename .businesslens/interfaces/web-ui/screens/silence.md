---
entities:
  - entity: silence
    shows: [ID, Matchers, Starts at, Ends at, Updated at, Created by, Comment, Annotations]
  - entity: alert
    shows: [Labels, Annotations, Starts at]
entryPoints:
  - web-ui: /#/silences/{silence-id}
references:
  - kind: code
    role: implementation
    target: ui/app/src/Views/SilenceView/Views.elm
  - kind: code
    role: implementation
    target: ui/app/src/Views/Shared/SilencePreview.elm
---

# Silence

One silence, its state and the alerts it currently affects.
