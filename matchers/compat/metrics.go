// Copyright 2023 The Prometheus Authors
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

package compat

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	OriginAPI    = "api"
	OriginConfig = "config"
)

var DefaultOrigins = []string{
	OriginAPI,
	OriginConfig,
}

var RegisteredMetrics = NewMetrics(prometheus.DefaultRegisterer)

type Metrics struct {
	Total             *prometheus.CounterVec
	DisagreeTotal     *prometheus.CounterVec
	IncompatibleTotal *prometheus.CounterVec
	InvalidTotal      *prometheus.CounterVec
}

func NewMetrics(r prometheus.Registerer) *Metrics {
	m := &Metrics{
		Total: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "alertmanager_matchers_parse_total",
			Help: "Total number of matcher inputs parsed, including invalid inputs.",
		}, []string{"origin"}),
		DisagreeTotal: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "alertmanager_matchers_disagree_total",
			Help: "Total number of matcher inputs which produce different parsings (disagreement).",
		}, []string{"origin"}),
		IncompatibleTotal: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "alertmanager_matchers_incompatible_total",
			Help: "Total number of matcher inputs that are incompatible with the UTF-8 parser.",
		}, []string{"origin"}),
		InvalidTotal: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "alertmanager_matchers_invalid_total",
			Help: "Total number of matcher inputs that could not be parsed.",
		}, []string{"origin"}),
	}
	return m
}
