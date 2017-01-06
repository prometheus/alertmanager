package format

import (
	"io"
	"time"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

type Config struct {
	Config      string            `json:"config"`
	ConfigJSON  config.Config     `json:configJSON`
	VersionINFO map[string]string `json:"versionInfo"`
	Uptime      time.Time         `json:"uptime"`
}

type Formatter interface {
	SetOutput(io.Writer)
	FormatSilences([]types.Silence) error
	FormatAlerts(model.Alerts) error
	FormatConfig(Config) error
}

var Formatters map[string]Formatter = map[string]Formatter{}
