package cli

import (
	"io/ioutil"
	"os"

	"github.com/alecthomas/kingpin"
	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/common/version"
	"gopkg.in/yaml.v2"
)

var (
	app             = kingpin.New("amtool", "Alertmanager CLI").DefaultEnvars()
	verbose         = app.Flag("verbose", "Verbose running information").Short('v').Bool()
	alertmanagerUrl = app.Flag("alertmanager.url", "Alertmanager to talk to").Required().URL()
	output          = app.Flag("output", "Output formatter (simple, extended, json)").Default("simple").Enum("simple", "extended", "json")
)

type amtoolConfigResolver struct {
	configData []map[string]string
}

func newConfigResolver() amtoolConfigResolver {
	files := []string{
		os.ExpandEnv("$HOME/.config/amtool/config.yml"),
		"/etc/amtool/config.yml",
	}

	resolver := amtoolConfigResolver{
		configData: make([]map[string]string, 0),
	}
	for _, f := range files {
		b, err := ioutil.ReadFile(f)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			panic(err)
		}
		var config map[string]string
		err = yaml.Unmarshal(b, &config)
		if err != nil {
			panic(err)
		}
		resolver.configData = append(resolver.configData, config)
	}

	return resolver
}

func (r amtoolConfigResolver) Resolve(key string, context *kingpin.ParseContext) ([]string, error) {
	for _, c := range r.configData {
		if v, ok := c[key]; ok {
			return []string{v}, nil
		}
	}
	return nil, nil
}

/*
View and modify the current Alertmanager state.

[Config File]

The alertmanager tool will read a config file in YAML format from one of two default config locations:
$HOME/.config/amtool/config.yml or /etc/amtool/config.yml

The accepted config options are as follows:

	alertmanager.url
		Set a default alertmanager url for each request

	author
		Set a default author value for new silences. If this argument is not specified then the username will be used

	comment_required
		Require a comment on silence creation

	output
		Set a default output type. Options are (simple, extended, json)

	date.format
		Sets the output format for dates. Defaults to 2006-01-02 15:04:05 MST
*/

// Execute parses the arguments and executes the corresponding command, it is
// called by cmd/amtool/main.main().
func Execute() {
	format.InitFormatFlags(app)

	app.Version(version.Print("amtool"))
	app.GetFlag("help").Short('h')

	app.Resolver(newConfigResolver())
	_, err := app.Parse(os.Args[1:])
	if err != nil {
		kingpin.Fatalf("%v\n", err)
	}
}
