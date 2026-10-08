---
kind: primary
routes:
  amtool: amtool
steps:
  - text: The Operator gives amtool template files and a template to render, optionally with data from a file
    kind: actor
    actor: operator
    entities: []
    contexts:
      amtool:
        place: amtool
  - text: amtool renders the template as text or HTML with that data, or with built-in sample data of one firing and one resolved entry
    kind: product
    actor: operator
    entities: []
    contexts:
      amtool:
        place: amtool
---

# Render a template with sample data

## Trigger

An operator writes or changes a notification template.

## Outcome

The operator sees what the template produces.
