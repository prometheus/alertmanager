package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"time"

	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/config"
	"github.com/spf13/cobra"
	//flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Config is the response type of alertmanager config endpoint
// Duped in cli/format needs to be moved to common/model
type Config struct {
	Config      string            `json:"config"`
	ConfigJSON  config.Config     `json:configJSON`
	VersionINFO map[string]string `json:"versionInfo"`
	Uptime      time.Time         `json:"uptime"`
}

type alertmanagerStatusResponse struct {
	Status    string `json:"status"`
	Data      Config `json:"data,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
	Error     string `json:"error,omitempty"`
}

// alertCmd represents the alert command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View the running config",
	Long: `View current config

The amount of output is controlled by the output selection flag:
	- Simple: Print just the running config
	- Extended: Print the running config as well as uptime and all version info
	- Json: Print entire config object as json`,
	RunE: queryConfig,
}

func init() {
	RootCmd.AddCommand(configCmd)
}

func fetchConfig() (Config, error) {
	configResponse := alertmanagerStatusResponse{}
	u, err := GetAlertmanagerURL()
	if err != nil {
		return Config{}, err
	}

	u.Path = path.Join(u.Path, "/api/v1/status")
	res, err := http.Get(u.String())
	if err != nil {
		return Config{}, err
	}

	defer res.Body.Close()
	decoder := json.NewDecoder(res.Body)

	err = decoder.Decode(&configResponse)
	if err != nil {
		return Config{}, err
	}

	return configResponse.Data, nil
}

func queryConfig(cmd *cobra.Command, args []string) error {
	config, err := fetchConfig()
	if err != nil {
		return err
	}

	formatter, found := format.Formatters[viper.GetString("output")]
	if !found {
		return errors.New("Unknown output formatter")
	}

	return formatter.FormatConfig(format.Config(config))
}
