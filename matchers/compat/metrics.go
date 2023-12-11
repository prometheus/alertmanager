package compat

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RegisteredMetrics = NewMetrics(prometheus.DefaultRegisterer)
)

const (
	OriginAPI    = "api"
	OriginConfig = "config"
)

var (
	DefaultOrigins = []string{
		OriginAPI,
		OriginConfig,
	}
)

type Metrics struct {
	Total             *prometheus.GaugeVec
	DisagreeTotal     *prometheus.GaugeVec
	IncompatibleTotal *prometheus.GaugeVec
	InvalidTotal      *prometheus.GaugeVec
}

func NewMetrics(r prometheus.Registerer) *Metrics {
	m := &Metrics{
		Total: promauto.With(r).NewGaugeVec(prometheus.GaugeOpts{
			Name: "alertmanager_matchers_parse_total",
			Help: "Total number of matcher inputs parsed, including invalid inputs.",
		}, []string{"origin"}),
		DisagreeTotal: promauto.With(r).NewGaugeVec(prometheus.GaugeOpts{
			Name: "alertmanager_matchers_disagree_total",
			Help: "Total number of matcher inputs which produce different parsings (disagreement).",
		}, []string{"origin"}),
		IncompatibleTotal: promauto.With(r).NewGaugeVec(prometheus.GaugeOpts{
			Name: "alertmanager_matchers_incompatible_total",
			Help: "Total number of matcher inputs that are incompatible with the UTF-8 parser.",
		}, []string{"origin"}),
		InvalidTotal: promauto.With(r).NewGaugeVec(prometheus.GaugeOpts{
			Name: "alertmanager_matchers_invalid_total",
			Help: "Total number of matcher inputs that could not be parsed.",
		}, []string{"origin"}),
	}
	return m
}
