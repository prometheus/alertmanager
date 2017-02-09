# Alertmanager UI

This is a re-write of the Alertmanager UI in [elm-lang](http://elm-lang.org/).

## Usage

### Filtering on the alerts page

By default, the alerts page only shows active (not silenced) alerts. Adding a query string containing will additionally show silenced alerts.

```
http://alertmanager/#/alerts?silenced=true
```

The alerts page can also be filtered by the receivers for a page. Receivers are
configured in Alertmanager's yaml configuration file.

```
http://alertmanager/#/alerts?receiver=backend
```

These filters can be used in conjunction.

## Requirements

- Go installed (https://golang.org/dl/)
- This repo is cloned into your `$GOPATH`
- Elm is installed (https://guide.elm-lang.org/get_started.html)
