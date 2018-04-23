package cli

import (
	"net/url"
	"os"

	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/alertmanager/cli/config"
	"github.com/prometheus/alertmanager/cli/format"
)

var (
	verbose         bool
	alertmanagerURL *url.URL
	output          string

	configFiles = []string{os.ExpandEnv("$HOME/.config/amtool/config.yml"), "/etc/amtool/config.yml"}
	legacyFlags = map[string]string{"comment_required": "require-comment"}
)

func requireAlertManagerURL(pc *kingpin.ParseContext) error {
	// Return without error if any help flag is set.
	for _, elem := range pc.Elements {
		f, ok := elem.Clause.(*kingpin.FlagClause)
		if !ok {
			continue
		}
		name := f.Model().Name
		if name == "help" || name == "help-long" || name == "help-man" {
			return nil
		}
	}
	if alertmanagerURL == nil {
		kingpin.Fatalf("required flag --alertmanager.url not provided")
	}
	return nil
}

func Execute() {
	var (
		app = kingpin.New("amtool", helpRoot).DefaultEnvars()
	)

	format.InitFormatFlags(app)

	app.Flag("verbose", "Verbose running information").Short('v').BoolVar(&verbose)
	app.Flag("alertmanager.url", "Alertmanager to talk to").URLVar(&alertmanagerURL)
	app.Flag("output", "Output formatter (simple, extended, json)").Short('o').Default("simple").EnumVar(&output, "simple", "extended", "json")
	app.Version(version.Print("amtool"))
	app.GetFlag("help").Short('h')
	app.UsageTemplate(kingpin.CompactUsageTemplate)

	resolver, err := config.NewResolver(configFiles, legacyFlags)
	if err != nil {
		kingpin.Fatalf("could not load config file: %v\n", err)
	}

	configureAlertCmd(app)
	configureSilenceCmd(app)
	configureCheckConfigCmd(app)
	configureConfigCmd(app)

	err = resolver.Bind(app, os.Args[1:])
	if err != nil {
		kingpin.Fatalf("%v\n", err)
	}

	_, err = app.Parse(os.Args[1:])
	if err != nil {
		kingpin.Fatalf("%v\n", err)
	}
}

const (
	helpRoot = `View and modify the current Alertmanager state.

Config File:
The alertmanager tool will read a config file in YAML format from one of two
default config locations: $HOME/.config/amtool/config.yml or
/etc/amtool/config.yml

All flags can be given in the config file, but the following are the suited for
static configuration:

	alertmanager.url
		Set a default alertmanager url for each request

	author
		Set a default author value for new silences. If this argument is not
		specified then the username will be used

	require-comment
		Bool, whether to require a comment on silence creation. Defaults to true

	output
		Set a default output type. Options are (simple, extended, json)

	date.format
		Sets the output format for dates. Defaults to "2006-01-02 15:04:05 MST"
`
)
