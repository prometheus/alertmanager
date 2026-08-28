INSERT INTO alertmanager_lifecycle_sink
WITH lifecycle_variants AS (
  SELECT
    events."@timestamp" AS event_time,
    events.instance,
    events."clusterPosition" AS cluster_position,
    json_get_json(events.data, 'alertmanagerStartupEvent') AS startup,
    json_get_json(events.data, 'alertmanagerShutdownEvent') AS shutdown
  FROM alertmanager_events AS events
),
lifecycle_events AS (
  SELECT
    event_time,
    instance,
    cluster_position,
    CASE WHEN startup IS NOT NULL THEN 'startup' ELSE 'shutdown' END AS event_type,
    CASE WHEN startup IS NOT NULL THEN startup ELSE shutdown END AS payload
  FROM lifecycle_variants
  WHERE startup IS NOT NULL OR shutdown IS NOT NULL
)
SELECT
  event_time,
  instance,
  cluster_position,
  event_type,
  CASE WHEN event_type = 'startup' THEN json_get_str(payload, 'version') END AS version,
  CASE WHEN event_type = 'startup' THEN json_get_str(payload, 'buildContext') END AS build_context,
  payload
FROM lifecycle_events;

INSERT INTO alertmanager_alerts_sink
WITH alert_variants AS (
  SELECT
    events."@timestamp" AS event_time,
    events.instance,
    events."clusterPosition" AS cluster_position,
    json_get_json(events.data, 'alertCreated') AS created,
    json_get_json(events.data, 'alertResolved') AS resolved,
    json_get_json(events.data, 'alertGrouped') AS grouped
  FROM alertmanager_events AS events
),
alert_events AS (
  SELECT
    event_time,
    instance,
    cluster_position,
    CASE
      WHEN created IS NOT NULL THEN 'created'
      WHEN resolved IS NOT NULL THEN 'resolved'
      ELSE 'grouped'
    END AS event_type,
    CASE
      WHEN created IS NOT NULL THEN json_get_json(created, 'alert')
      WHEN resolved IS NOT NULL THEN json_get_json(resolved, 'alert', 'details')
      ELSE json_get_json(grouped, 'alert', 'details')
    END AS alert,
    CASE
      WHEN resolved IS NOT NULL THEN json_get_json(resolved, 'groupInfo')
      WHEN grouped IS NOT NULL THEN json_get_json(grouped, 'groupInfo')
    END AS group_info,
    CASE
      WHEN created IS NOT NULL THEN created
      WHEN resolved IS NOT NULL THEN resolved
      ELSE grouped
    END AS payload
  FROM alert_variants
  WHERE created IS NOT NULL OR resolved IS NOT NULL OR grouped IS NOT NULL
)
SELECT
  event_time,
  instance,
  cluster_position,
  event_type,
  json_get_str(alert, 'name') AS alert_name,
  json_get_str(alert, 'fingerprint') AS alert_fingerprint,
  json_get_str(group_info, 'groupId') AS group_id,
  json_get_str(group_info, 'receiverName') AS receiver_name,
  json_get_str(alert, 'labels', 'severity') AS severity,
  json_get_str(alert, 'labels', 'service') AS service,
  json_get_str(alert, 'labels', 'cluster') AS cluster,
  json_get_str(alert, 'labels', 'team') AS team,
  json_get_json(alert, 'labels') AS labels,
  json_get_json(alert, 'annotations') AS annotations,
  payload
FROM alert_events;

INSERT INTO alertmanager_notifications_sink
WITH notification_events AS (
  SELECT
    events."@timestamp" AS event_time,
    events.instance,
    events."clusterPosition" AS cluster_position,
    json_get_json(events.data, 'notification') AS payload
  FROM alertmanager_events AS events
)
SELECT
  event_time,
  instance,
  cluster_position,
  json_get_str(payload, 'groupInfo', 'groupId') AS group_id,
  json_get_str(payload, 'groupInfo', 'receiverName') AS receiver_name,
  json_get_str(payload, 'reason') AS reason,
  json_get_str(payload, 'integration', 'name') AS integration_name,
  json_get_str(payload, 'integration', 'index') AS integration_index,
  json_get_str(payload, 'flushId') AS flush_id,
  payload
FROM notification_events
WHERE payload IS NOT NULL;

INSERT INTO alertmanager_silences_sink
WITH silence_variants AS (
  SELECT
    events."@timestamp" AS event_time,
    events.instance,
    events."clusterPosition" AS cluster_position,
    json_get_json(events.data, 'silenceCreated') AS created,
    json_get_json(events.data, 'silenceUpdated') AS updated,
    json_get_json(events.data, 'silenceMutedAlert') AS muted_alert
  FROM alertmanager_events AS events
),
silence_events AS (
  SELECT
    event_time,
    instance,
    cluster_position,
    CASE
      WHEN created IS NOT NULL THEN 'created'
      WHEN updated IS NOT NULL THEN 'updated'
      ELSE 'muted_alert'
    END AS event_type,
    CASE
      WHEN created IS NOT NULL THEN created
      WHEN updated IS NOT NULL THEN updated
      ELSE muted_alert
    END AS payload
  FROM silence_variants
  WHERE created IS NOT NULL OR updated IS NOT NULL OR muted_alert IS NOT NULL
)
SELECT
  event_time,
  instance,
  cluster_position,
  event_type,
  json_get_str(payload, 'silence', 'id') AS silence_id,
  json_get_str(payload, 'silence', 'createdBy') AS created_by,
  CASE
    WHEN event_type = 'muted_alert' THEN json_get_str(payload, 'mutedAlert', 'fingerprint')
  END AS muted_alert_fingerprint,
  payload
FROM silence_events;

INSERT INTO alertmanager_inhibitions_sink
WITH inhibition_events AS (
  SELECT
    events."@timestamp" AS event_time,
    events.instance,
    events."clusterPosition" AS cluster_position,
    json_get_json(events.data, 'inhibitionMutedAlert') AS payload
  FROM alertmanager_events AS events
)
SELECT
  event_time,
  instance,
  cluster_position,
  json_get_str(payload, 'mutedAlert', 'fingerprint') AS muted_alert_fingerprint,
  json_get_json(payload, 'inhibitRules') AS inhibit_rules,
  json_get_json(payload, 'inhibitingFingerprints') AS inhibiting_fingerprints,
  payload
FROM inhibition_events
WHERE payload IS NOT NULL;
