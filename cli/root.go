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
	output          = app.Flag("output", "Output formatter (simple, extended, json)").Short('o').Default("simple").Enum("simple", "extended", "json")

	// This contains a mapping from command path to the long-format help string
	// Separate subcommands with spaces, eg longHelpText["silence query"]
	longHelpText = make(map[string]string)
)

type amtoolConfigResolver struct {
	configData []map[string]string
}

func newConfigResolver() (amtoolConfigResolver, error) {
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
			return resolver, err
		}
		var config map[string]string
		err = yaml.Unmarshal(b, &config)
		if err != nil {
			return resolver, err
		}
		resolver.configData = append(resolver.configData, config)
	}

	return resolver, nil
}

func (r amtoolConfigResolver) Resolve(key string, context *kingpin.ParseContext) ([]string, error) {
	for _, c := range r.configData {
		if v, ok := c[key]; ok {
			return []string{v}, nil
		}
	}
	return nil, nil
}

// This function maps things which have previously had different names in the
// config file to their new names, so old configurations keep working
func backwardsCompatibilityResolver(key string) string {
	switch key {
	case "require-comment":
		return "comment_required"
	}
	return key
}

func init() {
	longHelpText["root"] = `View and modify the current Alertmanager state.

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
}

// Execute parses the arguments and executes the corresponding command, it is
// called by cmd/amtool/main.main().
func Execute() {
	format.InitFormatFlags(app)

	app.Version(version.Print("amtool"))
	app.GetFlag("help").Short('h')
	app.UsageTemplate(kingpin.CompactUsageTemplate)
	app.Flag("help-long", "Give more detailed help output").UsageAction(&kingpin.UsageContext{
		Template: longHelpTemplate,
		Vars:     map[string]interface{}{"LongHelp": longHelpText},
	}).Bool()

	configResolver, err := newConfigResolver()
	if err != nil {
		kingpin.Fatalf("could not load config file: %v\n", err)
	}
	// Use the same resolver twice, first for checking backwards compatibility,
	// then again for the new names. This order ensures that the newest wins, if
	// both old and new are present
	app.Resolver(
		kingpin.RenamingResolver(configResolver, backwardsCompatibilityResolver),
		configResolver,
	)

	_, err = app.Parse(os.Args[1:])
	if err != nil {
		kingpin.Fatalf("%v\n", err)
	}
}

const longHelpTemplate = `{{define "FormatCommands" -}}
{{range .FlattenedCommands -}}
{{if not .Hidden}}
  {{.CmdSummary}}
{{.Help|Wrap 4}}
{{if .Flags -}}
{{with .Flags|FlagsToTwoColumns}}{{FormatTwoColumnsWithIndent . 4 2}}{{end}}
{{end -}}
{{end -}}
{{end -}}
{{end -}}

{{define "FormatUsage" -}}
{{.AppSummary}}
{{if .Help}}
{{.Help|Wrap 0 -}}
{{end -}}

{{end -}}

{{if .Context.SelectedCommand -}}
{{T "usage:"}} {{.App.Name}} {{.App.FlagSummary}} {{.Context.SelectedCommand.CmdSummary}}

{{index .LongHelp .Context.SelectedCommand.FullCommand}}
{{else}}
{{T "usage:"}} {{template "FormatUsage" .App}}
{{index .LongHelp "root"}}
{{end}}
{{if .Context.Flags -}}
{{T "Flags:"}}
{{.Context.Flags|FlagsToTwoColumns|FormatTwoColumns}}
{{end -}}
{{if .Context.Args -}}
{{T "Args:"}}
{{.Context.Args|ArgsToTwoColumns|FormatTwoColumns}}
{{end -}}
{{if .Context.SelectedCommand -}}
{{if len .Context.SelectedCommand.Commands -}}
{{T "Subcommands:"}}
{{template "FormatCommands" .Context.SelectedCommand}}
{{end -}}
{{else if .App.Commands -}}
{{T "Commands:" -}}
{{template "FormatCommands" .App}}
{{end -}}
`
