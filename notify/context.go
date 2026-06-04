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
package notify

import (
	"context"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/pkg/labels"
)

// notifyKey defines a custom type with which a context is populated to
// avoid accidental collisions.
type notifyKey int

const (
	keyReceiverName notifyKey = iota
	keyRepeatInterval
	keyGroupLabels
	keyGroupKey
	keyFiringAlerts
	keyResolvedAlerts
	keyNow
	keyMuteTimeIntervals
	keyActiveTimeIntervals
	keyRouteID
	keyNflogStore
	keyNotificationReason
	keyMutedAlerts
	keyAggrGroupID
	keyFlushID
	keyGroupMatchers
)

// WithReceiverName populates a context with a receiver name.
func WithReceiverName(ctx context.Context, rcv string) context.Context {
	return context.WithValue(ctx, keyReceiverName, rcv)
}

// WithGroupKey populates a context with a group key.
func WithGroupKey(ctx context.Context, s string) context.Context {
	return context.WithValue(ctx, keyGroupKey, s)
}

// WithFiringAlerts populates a context with a slice of firing alerts.
func WithFiringAlerts(ctx context.Context, alerts []uint64) context.Context {
	return context.WithValue(ctx, keyFiringAlerts, alerts)
}

// WithResolvedAlerts populates a context with a slice of resolved alerts.
func WithResolvedAlerts(ctx context.Context, alerts []uint64) context.Context {
	return context.WithValue(ctx, keyResolvedAlerts, alerts)
}

// WithGroupLabels populates a context with grouping labels.
func WithGroupLabels(ctx context.Context, lset model.LabelSet) context.Context {
	return context.WithValue(ctx, keyGroupLabels, lset)
}

// WithNow populates a context with a now timestamp.
func WithNow(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, keyNow, t)
}

// WithRepeatInterval populates a context with a repeat interval.
func WithRepeatInterval(ctx context.Context, t time.Duration) context.Context {
	return context.WithValue(ctx, keyRepeatInterval, t)
}

// WithMuteTimeIntervals populates a context with a slice of mute time names.
func WithMuteTimeIntervals(ctx context.Context, mt []string) context.Context {
	return context.WithValue(ctx, keyMuteTimeIntervals, mt)
}

func WithActiveTimeIntervals(ctx context.Context, at []string) context.Context {
	return context.WithValue(ctx, keyActiveTimeIntervals, at)
}

func WithRouteID(ctx context.Context, routeID string) context.Context {
	return context.WithValue(ctx, keyRouteID, routeID)
}

func WithNotificationReason(ctx context.Context, reason NotifyReason) context.Context {
	return context.WithValue(ctx, keyNotificationReason, reason)
}

// RepeatInterval extracts a repeat interval from the context. Iff none exists, the
// second argument is false.
func RepeatInterval(ctx context.Context) (time.Duration, bool) {
	v, ok := ctx.Value(keyRepeatInterval).(time.Duration)
	return v, ok
}

// ReceiverName extracts a receiver name from the context. Iff none exists, the
// second argument is false.
func ReceiverName(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyReceiverName).(string)
	return v, ok
}

// GroupKey extracts a group key from the context. Iff none exists, the
// second argument is false.
func GroupKey(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyGroupKey).(string)
	return v, ok
}

// GroupLabels extracts grouping label set from the context. Iff none exists, the
// second argument is false.
func GroupLabels(ctx context.Context) (model.LabelSet, bool) {
	v, ok := ctx.Value(keyGroupLabels).(model.LabelSet)
	return v, ok
}

// Now extracts a now timestamp from the context. Iff none exists, the
// second argument is false.
func Now(ctx context.Context) (time.Time, bool) {
	v, ok := ctx.Value(keyNow).(time.Time)
	return v, ok
}

// FiringAlerts extracts a slice of firing alerts from the context.
// Iff none exists, the second argument is false.
func FiringAlerts(ctx context.Context) ([]uint64, bool) {
	v, ok := ctx.Value(keyFiringAlerts).([]uint64)
	return v, ok
}

// ResolvedAlerts extracts a slice of resolved alerts from the context.
// Iff none exists, the second argument is false.
func ResolvedAlerts(ctx context.Context) ([]uint64, bool) {
	v, ok := ctx.Value(keyResolvedAlerts).([]uint64)
	return v, ok
}

// MuteTimeIntervalNames extracts a slice of mute time names from the context. If and only if none exists, the
// second argument is false.
func MuteTimeIntervalNames(ctx context.Context) ([]string, bool) {
	v, ok := ctx.Value(keyMuteTimeIntervals).([]string)
	return v, ok
}

// ActiveTimeIntervalNames extracts a slice of active time names from the context. If none exists, the
// second argument is false.
func ActiveTimeIntervalNames(ctx context.Context) ([]string, bool) {
	v, ok := ctx.Value(keyActiveTimeIntervals).([]string)
	return v, ok
}

// RouteID extracts a RouteID from the context. Iff none exists, the
// // second argument is false.
func RouteID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyRouteID).(string)
	return v, ok
}

func NotificationReason(ctx context.Context) (NotifyReason, bool) {
	v, ok := ctx.Value(keyNotificationReason).(NotifyReason)
	return v, ok
}

// WithMutedAlerts populates a context with a set of muted alert hashes.
func WithMutedAlerts(ctx context.Context, alerts map[uint64]struct{}) context.Context {
	return context.WithValue(ctx, keyMutedAlerts, alerts)
}

// MutedAlerts extracts a set of muted alert hashes from the context.
func MutedAlerts(ctx context.Context) (map[uint64]struct{}, bool) {
	v, ok := ctx.Value(keyMutedAlerts).(map[uint64]struct{})
	return v, ok
}

// WithAggrGroupID populates a context with an aggregation group UUID.
func WithAggrGroupID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyAggrGroupID, id)
}

// AggrGroupID extracts an aggregation group UUID from the context.
func AggrGroupID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyAggrGroupID).(string)
	return v, ok
}

// WithFlushID populates a context with a flush identifier.
func WithFlushID(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, keyFlushID, id)
}

// FlushID extracts a flush identifier from the context.
func FlushID(ctx context.Context) (uint64, bool) {
	v, ok := ctx.Value(keyFlushID).(uint64)
	return v, ok
}

// WithGroupMatchers populates a context with the route's matchers.
func WithGroupMatchers(ctx context.Context, matchers labels.Matchers) context.Context {
	return context.WithValue(ctx, keyGroupMatchers, matchers)
}

// GroupMatchers extracts the route's matchers from the context.
func GroupMatchers(ctx context.Context) (labels.Matchers, bool) {
	v, ok := ctx.Value(keyGroupMatchers).(labels.Matchers)
	return v, ok
}

func WithNflogStore(ctx context.Context, store *nflog.Store) context.Context {
	return context.WithValue(ctx, keyNflogStore, store)
}

func NflogStore(ctx context.Context) (*nflog.Store, bool) {
	v, ok := ctx.Value(keyNflogStore).(*nflog.Store)
	return v, ok
}
