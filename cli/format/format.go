package format

import (
	"io"
	"time"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/types"
	"github.com/spf13/viper"
)

const DefaultDateFormat = "2006-01-02 15:04:05 MST"

// Config representation
// Need to get this moved to the prometheus/common/model repo having is duplicated here is smelly
type Config struct {
	ConfigYAML  string                 `json:"configYAML"`
	ConfigJSON  config.Config          `json:"configJSON"`
	MeshStatus  map[string]interface{} `json:"meshStatus"`
	VersionInfo map[string]string      `json:"versionInfo"`
	Uptime      time.Time              `json:"uptime"`
}

type MeshStatus struct {
	Name     string       `json:"name"`
	NickName string       `json:"nickName"`
	Peers    []PeerStatus `json:"peerStatus"`
}

type PeerStatus struct {
	Name     string `json:"name"`
	NickName string `json:"nickName"`
	UID      uint64 `uid`
}

// Formatter needs to be implemented for each new output formatter
type Formatter interface {
	SetOutput(io.Writer)
	FormatSilences([]types.Silence) error
	FormatAlerts([]*dispatch.APIAlert) error
	FormatConfig(Config) error
}

// Formatters is a map of cli argument name to formatter inferface object
var Formatters map[string]Formatter = map[string]Formatter{}

func FormatDate(input time.Time) string {
	dateformat := viper.GetString("date.format")
	return input.Format(dateformat)
}
