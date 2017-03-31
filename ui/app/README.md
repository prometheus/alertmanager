# Alertmanager UI

This is a re-write of the Alertmanager UI in [elm-lang](http://elm-lang.org/).

## Usage

### Filtering on the alerts page

By default, the alerts page only shows active (not silenced) alerts. Adding a
query string containing the following will additionally show silenced alerts.

```
http://alertmanager/#/alerts?silenced=true
```

The alerts page can also be filtered by the receivers for a page. Receivers are
configured in Alertmanager's yaml configuration file.

```
http://alertmanager/#/alerts?receiver=backend
```

Filtering based on label matchers is available. They can easily be added and
modified through the UI.

```
http://alertmanager/#/alerts?filter=%7Bseverity%3D%22warning%22%2C%20owner%3D%22backend%22%7D
```

These filters can be used in conjunction.

### Filtering on the silences page

Filtering based on label matchers is available. They can easily be added and
modified through the UI.

```
http://alertmanager/#/silences?filter=%7Bseverity%3D%22warning%22%2C%20owner%3D%22backend%22%7D
```

### Note on filtering via label matchers

Filtering via label matchers follows the same syntax and semantics as Prometheus.

A properly formatted filter is a set of label matchers joined by accepted
matching operators, surrounded by curly braces:

```
{foo="bar", baz=~"quu.*"}
```

Operators include:

- `=`
- `!=`
- `=~`
- `!~`

See the official documentation for additional information: https://prometheus.io/docs/querying/basics/#instant-vector-selectors
