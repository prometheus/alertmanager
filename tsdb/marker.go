package tsdb

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/schema"
)

// Marker subscribes to marker events from the MemMarker and stores them in the TSDB.
// It creates the ALERTS_MARKER metric in the TSDB.
type Marker struct {
	subscriber        *subscriber
	tsdb              *TSDB
	marker            *types.MemMarker
	metadata          schema.Metadata
	silencedMetadata  schema.Metadata
	inhibitedMetadata schema.Metadata
	logger            *slog.Logger
}

// NewMarker creates a new Marker instance.
func NewMarker(marker *types.MemMarker, tsdb *TSDB, logger *slog.Logger) (*Marker, error) {
	m := &Marker{
		tsdb:   tsdb,
		marker: marker,
		metadata: schema.Metadata{
			Name: "ALERTS_MARKER",
		},
		silencedMetadata: schema.Metadata{
			Name: "ALERTS_SILENCED",
		},
		inhibitedMetadata: schema.Metadata{
			Name: "ALERTS_INHIBITED",
		},
		logger: logger,
	}

	m.subscriber = newSubscriber("marker", m.run, logger)

	return m, nil
}

// Run starts the Marker subscriber.
func (m *Marker) Run() {
	m.subscriber.Run()
}

// Stop stops the Marker subscriber.
func (m *Marker) Stop() {
	m.subscriber.Stop()
}

func (m *Marker) run(ctx context.Context) {
	events, done := m.marker.Subscribe("tsdb-marker")
	defer close(done)

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			if event.Status != nil {
				m.append(event.Fingerprint, event.Status)
			}
		}
	}
}

// append appends a marker event to the TSDB.
func (m *Marker) append(fingerprint model.Fingerprint, status *types.AlertStatus) error {
	ts := time.Now().UnixMilli()
	appender := m.tsdb.tsdb.Appender(context.Background())

	// Write 1 for current state, StaleNaN for all other states
	states := []types.AlertState{
		types.AlertStateActive,
		types.AlertStateSuppressed,
		types.AlertStateResolved,
	}
	for _, s := range states {
		lbls := labels.NewBuilder(labels.EmptyLabels())
		m.metadata.SetToLabels(lbls)
		lbls.Set("fingerprint", fingerprint.String())
		lbls.Set("alertstate", string(s))
		var val float64
		if s == status.State {
			val = 1
		} else {
			val = math.Float64frombits(value.StaleNaN)
		}

		_, err := appender.Append(0, lbls.Labels(), ts, val)
		if err != nil {
			appender.Rollback()
			return err
		}
	}

	if status.InhibitedBy != nil {
		if err := m.appendSuppressed(ts, m.inhibitedMetadata, fingerprint, status.InhibitedBy); err != nil {
			m.logger.Error("failed to append inhibited marker", "error", err)
		}
	}

	if status.SilencedBy != nil {
		if err := m.appendSuppressed(ts, m.silencedMetadata, fingerprint, status.SilencedBy); err != nil {
			m.logger.Error("failed to append silenced marker", "error", err)
		}
	}

	return appender.Commit()
}

func (m *Marker) appendSuppressed(ts int64, metadata schema.Metadata, fingerprint model.Fingerprint, by []string) error {
	appender := m.tsdb.tsdb.Appender(context.Background())
	lbls := labels.NewBuilder(labels.EmptyLabels())
	metadata.SetToLabels(lbls)
	lbls.Set("fingerprint", fingerprint.String())
	for _, b := range by {
		lbls.Set("by", b)
		_, err := appender.Append(0, lbls.Labels(), ts, 1)
		if err != nil {
			appender.Rollback()
			return err
		}
	}
	return appender.Commit()
}
