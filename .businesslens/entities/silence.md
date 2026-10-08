---
domain: silences
references:
  - kind: code
    role: implementation
    target: silence/silence.go
  - kind: code
    role: implementation
    target: silence/state.go
  - kind: code
    role: implementation
    target: api/v2/api.go#postSilencesHandler
---

# Silence

A time-bound instruction to mute every alert its matchers select. Silences are
kept on disk, survive restarts and are shared with every peer in a cluster.

## Information kept

- **ID** — the identifier Alertmanager assigns when it creates the silence
- **Matchers** — label matchers (equal, not equal, regular expression or negated regular expression), all of which a muted alert must satisfy
- **Starts at** — when the silence starts muting
- **Ends at** — when the silence stops muting
- **Created by** — who the silence says created it
- **Comment** — why the silence exists
- **Annotations** — further key and value pairs kept with the silence
- **Updated at** — when the silence last changed

## States

### Active

Between its start and end: it mutes the alerts it matches.

### Pending

Its start time is still in the future.

### Expired

Its end time has passed or someone expired it. It is kept for the data
retention period and then removed.
