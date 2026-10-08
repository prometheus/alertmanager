---
appliesTo:
  - { type: capability, id: send-alerts }
  - { type: capability, id: browse-alerts }
  - { type: capability, id: browse-silences }
  - { type: capability, id: browse-receivers }
  - { type: capability, id: create-silence }
  - { type: capability, id: edit-silence }
  - { type: capability, id: start-alertmanager }
  - { type: capability, id: reload-configuration }
  - { type: capability, id: check-configuration }
references:
  - kind: code
    role: implementation
    target: matcher/compat/parse.go#UTF8MatchersParser
---

# Matchers are read with the UTF-8 syntax only

Label names may use any UTF-8 characters, and matcher text the UTF-8 syntax
cannot read is refused without falling back to the classic syntax.
