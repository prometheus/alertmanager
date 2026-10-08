---
appliesTo:
  - { type: capability, id: send-notifications }
  - { type: entity, id: notification }
references:
  - kind: code
    role: implementation
    target: notify/retry_stage.go
---

# A failed notification is retried only within the group interval

A notification that fails with an error retrying can fix is retried with a
growing delay, or the delay the service asks for, until the group interval
runs out. An error retrying cannot fix ends it at once, and only an accepted
notification is recorded as sent.
