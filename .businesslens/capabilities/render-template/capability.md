---
availability:
  - { place: amtool }
references:
  - kind: code
    role: implementation
    target: cli/template_render.go
  - kind: code
    role: implementation
    target: template/template.go
---

# Render template

An operator renders a notification template from template files without a
running Alertmanager, with their own data or with built-in sample data.
