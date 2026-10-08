---
appliesTo:
  - { type: capability, id: inhibit-alerts }
  - { type: entity, id: inhibition-rule }
references:
  - kind: code
    role: implementation
    target: inhibit/inhibit.go
---

# An alert is inhibited only by a firing alert sharing its equal labels

An inhibition rule mutes a target alert only while a firing source alert has
the same values for every equal label of the rule, a label missing from both
counting as equal, and never through a source alert that also matches the
rule's target matchers.
