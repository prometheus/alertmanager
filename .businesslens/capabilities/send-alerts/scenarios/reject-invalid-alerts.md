---
kind: validation
routes:
  api: API
steps:
  - text: The Alert generator sends a batch in which one entry has no labels, an invalid label name or an end before its start
    kind: actor
    actor: alert-generator
    entities: []
    contexts:
      api:
        place: api
  - text: The Product keeps every valid Alert of the batch and refuses the invalid entries
    kind: product
    actor: alert-generator
    entities:
      - { entity: alert, effect: creates, to: Firing, facts: [Labels, Annotations, Starts at, Ends at, Updated at, Generator URL, Fingerprint] }
    contexts:
      api:
        place: api
  - text: The request fails as a bad request listing what is wrong with each refused entry
    kind: condition
    entities: []
    contexts:
      api:
        place: api
---

# Reject invalid alerts

## Trigger

An alert generator sends alerts that are not well formed.

## Outcome

The valid alerts are kept, the invalid ones are not, and the alert generator
is told which entries were refused and why; the kept alerts stay kept.

## Edge cases

- Which label names are valid depends on the matcher parsing mode in use.
