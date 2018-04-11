package cli

import (
	"context"
	"errors"

	"github.com/alecthomas/kingpin"
	"github.com/prometheus/client_golang/api"

	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/client"
)

// configCmd represents the config command
func configureConfigCmd(app *kingpin.Application, longHelpText map[string]string) {
	app.Command("config", "View the running config").Action(queryConfig)

	longHelpText["config"] = `View current config
The amount of output is controlled by the output selection flag:
	- Simple: Print just the running config
	- Extended: Print the running config as well as uptime and all version info
	- Json: Print entire config object as json`
}

func queryConfig(element *kingpin.ParseElement, ctx *kingpin.ParseContext) error {
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
