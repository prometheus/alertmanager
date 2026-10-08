---
entities:
  - entity: browser-settings
    shows: [First day of the week]
    collects: [First day of the week]
entryPoints:
  - web-ui: /#/settings
references:
  - kind: code
    role: implementation
    target: ui/app/src/Views/Settings/Views.elm
  - kind: code
    role: implementation
    target: ui/app/src/Views/Settings/Updates.elm
---

# Settings

The preferences this browser keeps for the web UI.
