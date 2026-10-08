---
kind: validation
routes:
  amtool: amtool
steps:
  - text: The Operator checks a config file with a mistake in it
    kind: actor
    actor: operator
    entities: []
    contexts:
      amtool:
        place: amtool
  - text: amtool reports the file as failed with the reason and exits with an error
    kind: product
    actor: operator
    entities: []
    contexts:
      amtool:
        place: amtool
---

# Report an invalid file

## Trigger

An operator checks a configuration file that would not load.

## Outcome

The operator knows what to fix before reloading.
