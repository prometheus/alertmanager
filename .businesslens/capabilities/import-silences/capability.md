---
domain: silences
availability:
  - { place: amtool }
references:
  - kind: code
    role: implementation
    target: cli/silence_import.go#silenceImportCmd.bulkImport
  - kind: code
    role: implementation
    target: cli/silence_import.go#addSilenceWorker
---

# Import silences

An operator loads many silences at once into an Alertmanager from a file or from
standard input, typically silences exported from another Alertmanager with
amtool.
