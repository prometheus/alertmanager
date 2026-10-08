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
    target: matcher/compat/parse.go#FallbackMatchersParser
---

# Matchers are read as UTF-8 matchers, falling back to the classic syntax

Label names may use any UTF-8 characters. Matcher text the UTF-8 syntax
cannot read, but the classic syntax can, is still accepted with a logged
warning that suggests the compatible form; where both syntaxes read the text
differently, the classic reading applies.
