---
kind: configuration
of: business-rule
takesEffect: When Alertmanager or amtool starts, from the features enabled on its command line.
stability: The mode holds for the life of the process; a configuration reload does not change it, and changing it needs a restart. Silences already kept keep their matchers.
alternatives:
  - id: matchers-parse-with-fallback
    selectedWhen: Neither the `classic-mode` nor the `utf8-strict-mode` feature is enabled. This is the default.
  - id: matchers-parse-classically
    selectedWhen: The `classic-mode` feature is enabled.
  - id: matchers-parse-as-utf8-only
    selectedWhen: The `utf8-strict-mode` feature is enabled. Alertmanager refuses to start with both features enabled.
references:
  - kind: code
    role: implementation
    target: matcher/compat/parse.go#InitFromFlags
  - kind: code
    role: implementation
    target: featurecontrol/featurecontrol.go
---

# Matcher parsing

Alertmanager is moving label matchers from the classic syntax to a UTF-8
syntax; operators choose how strictly matcher text in the configuration, the
API and amtool is read while both are supported.
