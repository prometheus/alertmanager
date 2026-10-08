---
kind: primary
routes:
  amtool: amtool
steps:
  - text: The Operator checks one or more config files with amtool
    kind: actor
    actor: operator
    entities: []
    contexts:
      amtool:
        place: amtool
  - text: amtool loads each file and its templates and reports success with a summary of what it defines
    kind: product
    actor: operator
    entities: []
    contexts:
      amtool:
        place: amtool
---

# Check a valid file

## Trigger

An operator has edited a configuration file and wants to know it will load.

## Outcome

The operator knows each file is valid.

## Edge cases

- With no file named, amtool reads the configuration from standard input.
