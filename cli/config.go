package cli

import (
	"context"
	"errors"

	"github.com/prometheus/client_golang/api"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/client"
)

const configHelp = `View current config.

The amount of output is controlled by the output selection flag:
	- Simple: Print just the running config
	- Extended: Print the running config as well as uptime and all version info
	- Json: Print entire config object as json
`

// configCmd represents the config command
func configureConfigCmd(app *kingpin.Application) {
	app.Command("config", configHelp).Action(queryConfig).PreAction(requireAlertManagerURL)

}

func queryConfig(ctx *kingpin.ParseContext) error {
	c, err := api.NewClient(api.Config{Address: alertmanagerURL.String()})
	if err != nil {
		return err
	}
	statusAPI := client.NewStatusAPI(c)
	status, err := statusAPI.Get(context.Background())
	if err != nil {
		return err
	}

	formatter, found := format.Formatters[output]
	if !found {
		return errors.New("unknown output formatter")
	}

	return formatter.FormatConfig(status)
}
