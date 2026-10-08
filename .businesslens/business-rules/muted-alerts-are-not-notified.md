---
appliesTo:
  - { type: capability, id: send-notifications }
  - { type: capability, id: silence-alerts }
  - { type: capability, id: inhibit-alerts }
references:
  - kind: code
    role: implementation
    target: notify/notify.go
  - kind: code
    role: implementation
    target: notify/mute.go
---

# Silenced, inhibited and time-muted alerts are left out of notifications

An alert an active silence or an inhibiting alert mutes is not part of any
notification, and a group muted by its route's time intervals sends nothing.
Muted alerts stay listed in the web UI and API with what mutes them, and
routing still places them in their groups.
