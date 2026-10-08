---
scope: The Alertmanager server, its HTTP API, the web UI served at its root and amtool.
method: Static reading of source code, the OpenAPI specification and documentation; nothing was built or run.
covered:
  - description: Alert intake, storage, merging, limits and clean-up.
    paths: [alert/, provider/, store/, limit/, marker/, labelset/]
  - description: Routing tree, alert grouping and the dispatcher.
    paths: [dispatch/]
  - description: Notification pipeline, deduplication, retries, time muting and the notification log.
    paths: [notify/notify.go, notify/dedup_stage.go, notify/retry_stage.go, notify/mute.go, notify/set_notifies_stage.go, notify/cluster_stages.go, notify/util.go, nflog/]
  - description: Inhibition engine.
    paths: [inhibit/]
  - description: Silence store, validation, updates, expiry, muting and gossip merge.
    paths: [silence/]
  - description: Time interval evaluation.
    paths: [timeinterval/]
  - description: Configuration parsing, validation, secrets and reload coordination.
    paths: [config/config.go, config/coordinator.go, config/common/]
  - description: Process lifecycle, startup, reload wiring, shutdown, listeners and command-line flags.
    paths: [app/, cmd/alertmanager/]
  - description: HA clustering and gossip of silences and the notification log.
    paths: [cluster/]
  - description: HTTP API v2 handlers, its OpenAPI specification, the API mux and the API v1 deprecation responder.
    paths: [api/v2/api.go, api/v2/compat.go, api/v2/openapi.yaml, api/api.go, api/v1_deprecation_router.go]
  - description: Health, readiness, reload and metrics endpoints.
    paths: [httpserver/]
  - description: Web UI pages for alerts, silences, status and settings, and UI serving.
    paths: [ui/app/src/, ui/web.go]
  - description: amtool command-line tool.
    paths: [cli/, cmd/amtool/]
  - description: Notification template data and rendering.
    paths: [template/template.go, template/default.tmpl]
  - description: Event recorder and its Kafka transport.
    paths: [eventrecorder/, kafka/, proto/eventrecorder/]
  - description: Matcher parsing and the feature flags that select its mode.
    paths: [matcher/, pkg/labels/, featurecontrol/]
exclusions:
  - description: Generated API server, client, model and UI data code.
    paths: [api/v2/models/, api/v2/restapi/, api/v2/client/, ui/app/src/Data/]
  - description: Generated protocol buffer code.
    paths: [cluster/clusterpb/, nflog/nflogpb/, silence/silencepb/, eventrecorder/events/v2/, api/status/v3alpha/]
  - description: Tracing, API instrumentation and the monitoring mixin.
    paths: [tracing/, api/metrics/, doc/alertmanager-mixin/]
  - description: Build, release, CI and repository tooling.
    paths: [Makefile, Makefile.common, .promu.yml, Dockerfile, Dockerfile.distroless, scripts/, internal/tools/, .github/, buf.yaml, buf.gen.yaml, template/Makefile, template/package.json, template/package-lock.json, template/inline-css.js]
  - description: Integration and acceptance test harnesses and examples.
    paths: [test/, examples/, doc/examples/]
  - description: Shared filesystem helper for embedded assets.
    paths: [pkg/modtimevfs/]
unmapped:
  - description: Per-integration notifier behaviour and settings for each receiver type.
    paths: [notify/discord/, notify/email/, notify/incidentio/, notify/jira/, notify/mattermost/, notify/msteams/, notify/msteamsv2/, notify/opsgenie/, notify/pagerduty/, notify/pushover/, notify/rocketchat/, notify/slack/, notify/sns/, notify/telegram/, notify/victorops/, notify/webex/, notify/webhook/, notify/wechat/, config/receiver/]
  - description: New React web UI served under /ui/.
    paths: [ui/mantine-ui/]
  - description: Experimental ConnectRPC status API with gRPC health and reflection.
    paths: [api/connect/, proto/api/status/]
  - description: Default notification templates per integration and the email template.
    paths: [template/email.html, template/email.tmpl]
limitations:
  - description: HTTP basic authentication and TLS are enforced by the exporter-toolkit library, whose code is not in this repository.
    paths: [app/listen.go]
---

# Coverage
