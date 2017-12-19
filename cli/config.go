package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/alecthomas/kingpin"
	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/config"
)

// Config is the response type of alertmanager config endpoint
// Duped in cli/format needs to be moved to common/model
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

type alertmanagerStatusResponse struct {
	Status    string `json:"status"`
	Data      Config `json:"data,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
	Error     string `json:"error,omitempty"`
}

// configCmd represents the config command
var configCmd = app.Command("config", "View the running config").Action(queryConfig)

func init() {
	longHelpText["config"] = `View current config
The amount of output is controlled by the output selection flag:
	- Simple: Print just the running config
	- Extended: Print the running config as well as uptime and all version info
	- Json: Print entire config object as json`
}

func fetchConfig() (Config, error) {
	configResponse := alertmanagerStatusResponse{}

	u := GetAlertmanagerURL("/api/v1/status")
	res, err := http.Get(u.String())
	if err != nil {
		return Config{}, err
	}

	defer res.Body.Close()

	err = json.NewDecoder(res.Body).Decode(&configResponse)
	if err != nil {
		return configResponse.Data, err
	}

	if configResponse.Status != "success" {
		return Config{}, fmt.Errorf("[%s] %s", configResponse.ErrorType, configResponse.Error)
	}

	return configResponse.Data, nil
}

func queryConfig(element *kingpin.ParseElement, ctx *kingpin.ParseContext) error {
	config, err := fetchConfig()
	if err != nil {
		return err
	}

	formatter, found := format.Formatters[*output]
	if !found {
		return errors.New("unknown output formatter")
	}

	c := format.Config(config)

	return formatter.FormatConfig(c)
}
