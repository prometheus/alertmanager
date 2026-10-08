---
appliesTo:
  - { type: capability, id: create-silence }
  - { type: capability, id: edit-silence }
  - { type: capability, id: import-silences }
references:
  - kind: code
    role: implementation
    target: silence/silence.go
  - kind: doc
    role: intent
    target: docs/configuration.md
---

# Silences stay within the configured number and size limits

When the operator sets a maximum number of silences, counting expired ones
still kept, no new silence is kept once it is reached, including the new
silence an edit would create; when the operator sets a maximum silence size,
no larger silence is kept or changed. Both limits are off unless set, and
silences received from peers are kept whatever the limits.

## Rationale

Silences are kept in memory, written to disk and gossiped to every peer, so an
unbounded number or size of silences can exhaust an instance.
