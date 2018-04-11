package cli

import (
	"net/url"

	"github.com/alecthomas/kingpin"
)

var (
	verbose         bool
	alertmanagerURL *url.URL
	output          string
)

func ConfigureCommands(app *kingpin.Application, longHelpText map[string]string) {
	app.Flag("verbose", "Verbose running information").Short('v').BoolVar(&verbose)
	app.Flag("alertmanager.url", "Alertmanager to talk to").Required().URLVar(&alertmanagerURL)
	app.Flag("output", "Output formatter (simple, extended, json)").Short('o').Default("simple").EnumVar(&output, "simple", "extended", "json")

	configureAlertCmd(app, longHelpText)
	configureSilenceCmd(app, longHelpText)
	configureCheckConfigCmd(app, longHelpText)
	configureConfigCmd(app, longHelpText)
}
