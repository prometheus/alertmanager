---
references:
  - kind: code
    role: implementation
    target: config/common/inhibitrule.go#InhibitRule
  - kind: code
    role: implementation
    target: inhibit/inhibit.go
---

# Inhibition rule

A rule that mutes target alerts while a matching source alert is firing, such
as muting every alert about a cluster while an alert says the whole cluster is
unreachable.

## Information kept

- **Name** — an optional name for the rule
- **Source matchers** — the matchers a firing alert must satisfy to inhibit
- **Target matchers** — the matchers an alert must satisfy to be inhibited
- **Equal labels** — the labels whose values the source and target alerts must share
