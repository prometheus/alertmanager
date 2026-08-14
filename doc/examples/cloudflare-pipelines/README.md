# Send event recorder events to Cloudflare Pipelines

The event recorder can send batched JSON events to a Cloudflare Pipelines
stream through its webhook output.

Create a structured stream matching the event recorder's top-level JSON
envelope:

```sh
npx wrangler pipelines streams create alertmanager_events \
  --schema-file doc/examples/cloudflare-pipelines/stream-schema.json
```

Cloudflare stream schemas and pipeline SQL cannot be modified after creation.
Delete and recreate the stream and pipeline when changing either file.

Create the R2 bucket and enable R2 Data Catalog on it:

```sh
npx wrangler r2 bucket create alertmanager-events
npx wrangler r2 bucket catalog enable alertmanager-events
```

Create one R2 Data Catalog sink for each destination table. Replace the bucket,
namespace, and catalog token values as needed:

```sh
for name in lifecycle alerts notifications silences inhibitions; do
  npx wrangler pipelines sinks create alertmanager_${name}_sink \
    --type r2-data-catalog \
    --bucket alertmanager-events \
    --namespace alertmanager \
    --table ${name} \
    --catalog-token YOUR_CATALOG_TOKEN \
    --roll-interval 60
done
```

`YOUR_CATALOG_TOKEN` is the value of an R2 API token with **Admin Read &
Write** permission. Create one from **R2 Object Storage** > **Manage API
tokens** > **Create Account API token** in the Cloudflare dashboard. See
[Create an API token](https://developers.cloudflare.com/pipelines/getting-started/#1-create-an-api-token)
in the Cloudflare Pipelines documentation.

Create a pipeline that fans the stream out to those sinks:

```sh
npx wrangler pipelines create alertmanager_events_pipeline \
  --sql-file doc/examples/cloudflare-pipelines/pipeline.sql
```

The SQL stores process lifecycle, alerts, notifications, silences, and
inhibitions in separate tables. Alert rows promote the `severity`, `service`,
`cluster`, and `team` labels into columns while retaining the complete `labels`
and `annotations` as JSON objects keyed by label name. Each row also includes
commonly queried identifiers and the complete event-specific JSON payload.
Protobuf 64-bit integer fields such as fingerprints, flush IDs, and integration
indexes are strings in protojson and are therefore stored as strings.

For example, query an arbitrary alert label with R2 SQL using
`json_get_str(labels, 'label_name')`.

Configure Alertmanager with the stream's HTTP ingestion endpoint:

```yaml
event_recorder:
  webhook_outputs:
  - url_file: /etc/alertmanager/cloudflare-stream-url
    batch: true
    http_config:
      authorization:
        credentials_file: /etc/alertmanager/cloudflare-stream-token
```

The URL file must contain the stream's full ingestion endpoint, such as
`https://<stream-id>.ingest.cloudflare.com`.

Start Alertmanager with `--enable-feature=event-recorder`. When HTTP ingestion
authentication is enabled, the token must have the `Workers Pipeline Send`
permission. Create one by following
[Create an API token](https://developers.cloudflare.com/fundamentals/api/get-started/create-token/)
in the Cloudflare documentation. This is a different token from the sink catalog
token above: the catalog token lets the sinks write tables to R2, while this one
only lets Alertmanager send events to the stream.

The event recorder schema encodes labels, annotations, and group labels as JSON
objects keyed by name. The `data` field
remains JSON so the stream accepts every event variant and future additions to
the event recorder schema. The other envelope fields are validated before the
SQL extracts the event-specific protobuf `oneof` from `data`.

See the Cloudflare documentation for [managing streams](https://developers.cloudflare.com/pipelines/streams/manage-streams/)
and [fan-out pipelines](https://developers.cloudflare.com/pipelines/pipelines/manage-pipelines/#route-one-stream-to-multiple-tables).
