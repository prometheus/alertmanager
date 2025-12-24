package tsdb

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/schema"
)

// Alerts subscribes to alerts from the Alerts provider and stores them in the TSDB.
// It creates the ALERTS metric in the TSDB.
type Alerts struct {
	subscriber *subscriber
	tsdb       *TSDB
	alerts     provider.Alerts
	metadata   schema.Metadata
	logger     *slog.Logger
}

// NewAlerts creates a new Alerts instance.
func NewAlerts(alerts provider.Alerts, tsdb *TSDB, logger *slog.Logger) (*Alerts, error) {
	metadata := schema.Metadata{
		Name: "ALERTS",
	}

	a := &Alerts{
		tsdb:     tsdb,
		alerts:   alerts,
		metadata: metadata,
		logger:   logger,
	}

	a.subscriber = newSubscriber("alerts", a.run, logger)

	return a, nil
}

// Run starts the Alerts subscriber.
func (a *Alerts) Run() {
	a.subscriber.Run()
}

// Stop stops the Alerts subscriber.
func (a *Alerts) Stop() {
	a.subscriber.Stop()
}

func (a *Alerts) run(ctx context.Context) {
	it := a.alerts.Subscribe("tsdb")
	defer it.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case alert := <-it.Next():
			if err := it.Err(); err != nil {
				a.logger.Error("Error iterating alerts", "err", err)
				continue
			}
			traceCtx := context.Background()
			// if alert.Header != nil {
			// 	traceCtx = a.propagator.Extract(traceCtx, propagation.MapCarrier(alert.Header))
			// }
			a.append(traceCtx, alert.Data)
		}
	}
}

// append appends an alert to the TSDB.
func (a *Alerts) append(ctx context.Context, alert *types.Alert) error {
	appender := a.tsdb.tsdb.Appender(ctx)
	now := time.Now()
	ts := now.UnixMilli()

	lbls := labels.NewBuilder(labels.EmptyLabels())
	a.metadata.SetToLabels(lbls)

	for k, v := range alert.Labels {
		lbls.Set(string(k), string(v))
	}

	lbls.Set("fingerprint", alert.Labels.Fingerprint().String())
	lbls.Set("alertstate", string(alert.StatusAt(now)))

	_, err := appender.Append(0, lbls.Labels(), ts, 1)
	if err != nil {
		appender.Rollback()
		return err
	}
	return appender.Commit()
}
