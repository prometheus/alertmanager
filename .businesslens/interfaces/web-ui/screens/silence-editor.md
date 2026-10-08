---
entities:
  - entity: silence
    shows: [Matchers, Starts at, Ends at, Created by, Comment, Annotations]
    collects: [Matchers, Starts at, Ends at, Created by, Comment, Annotations]
  - entity: alert
    shows: [Labels, Annotations, Starts at]
  - entity: browser-settings
    shows: [Default creator]
entryPoints:
  - web-ui: /#/silences/new
references:
  - kind: code
    role: implementation
    target: ui/app/src/Views/SilenceForm/Views.elm
  - kind: code
    role: implementation
    target: ui/app/src/Views/SilenceForm/Updates.elm
---

# Silence editor

Where an operator writes a new silence or changes an existing one: its
matchers, its start, end or duration, its creator, its comment and its
annotations, with a preview of the alerts it would affect. A new silence may
arrive prefilled from an alert, from the alert filter or from an expired
silence.
