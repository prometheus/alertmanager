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
    target: matcher/compat/parse.go#ClassicMatchersParser
---

# Matchers are read with the classic syntax only

Matcher text is read with the classic syntax, and label names outside the
classic label name characters are refused.
