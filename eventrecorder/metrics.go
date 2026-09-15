// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package eventrecorder

import "github.com/prometheus/client_golang/prometheus"

// metrics holds Prometheus metrics for the event recorder.  The struct
// is internal — individual counters are passed to output constructors
// that need them (e.g. outputDrops to webhook and kafka, kafkaProduceErrors
// to kafka).
type metrics struct {
	eventsRecorded            *prometheus.CounterVec
	eventRecorderBytesWritten *prometheus.CounterVec
	eventsDropped             *prometheus.CounterVec
	eventSerializeErrors      *prometheus.CounterVec
	outputDrops               *prometheus.CounterVec
	kafkaProduceErrors        *prometheus.CounterVec
}

// newMetrics builds and (if r is non-nil) registers every metric the
// event recorder exposes.  Pass nil for r in tests to obtain a metric
// set that is not registered against a global registry.
func newMetrics(r prometheus.Registerer) *metrics {
	eventsRecorded := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_events_recorded_total",
		Help: "The total number of events recorded by the event recorder.",
	}, []string{"event_type", "output", "result"})

	eventRecorderBytesWritten := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_event_recorder_bytes_written_total",
		Help: "The total number of bytes written to the event recorder.",
	}, []string{"event_type", "output"})

	eventsDropped := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_events_dropped_total",
		Help: "The total number of events dropped due to a full queue.",
	}, []string{"event_type"})

	eventSerializeErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_event_serialize_errors_total",
		Help: "The total number of events that failed to serialize.",
	}, []string{"event_type"})

	// outputDrops is incremented when an output's local delivery buffer
	// is full and an event has to be dropped before reaching the wire.
	// Replaces the legacy alertmanager_event_webhook_drops_total metric
	// and is shared by the webhook and kafka outputs.
	outputDrops := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_event_output_drops_total",
		Help: "The total number of events dropped by an output due to a full local buffer.",
	}, []string{"output"})

	kafkaProduceErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_event_kafka_produce_errors_total",
		Help: "The total number of Kafka produce attempts that failed.",
	}, []string{"output", "error_type"})

	if r != nil {
		r.MustRegister(eventsRecorded, eventRecorderBytesWritten, eventsDropped,
			eventSerializeErrors, outputDrops, kafkaProduceErrors)
	}

	return &metrics{
		eventsRecorded:            eventsRecorded,
		eventRecorderBytesWritten: eventRecorderBytesWritten,
		eventsDropped:             eventsDropped,
		eventSerializeErrors:      eventSerializeErrors,
		outputDrops:               outputDrops,
		kafkaProduceErrors:        kafkaProduceErrors,
	}
}
