package format

import (
	"io"
	"time"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/spf13/viper"
)

const DefaultDateFormat = "2006-01-02 15:04:05 MST"

// Config representation
// Need to get this moved to the prometheus/common/model repo having is duplicated here is smelly
type Config struct {
	Config      string            `json:"config"`
	ConfigJSON  config.Config     `json:configJSON`
	VersionINFO map[string]string `json:"versionInfo"`
	Uptime      time.Time         `json:"uptime"`
}

// Formatter needs to be implemented for each new output formatter
type Formatter interface {
	SetOutput(io.Writer)
	FormatSilences([]types.Silence) error
	FormatAlerts(model.Alerts) error
	FormatConfig(Config) error
}

// Formatters is a map of cli argument name to formatter inferface object
var Formatters map[string]Formatter = map[string]Formatter{}

func FormatDate(input time.Time) string {
	dateformat := viper.GetString("date.format")
	return input.Format(dateformat)
}
