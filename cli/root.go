// Copyright 2018 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"net/url"
	"os"
	"path"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/prometheus/common/version"
	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/alertmanager/api/v2/client"
	"github.com/prometheus/alertmanager/cli/config"
	"github.com/prometheus/alertmanager/cli/format"

	clientruntime "github.com/go-openapi/runtime/client"
)

var (
	verbose         bool
	alertmanagerURL *url.URL
	output          string
	timeout         time.Duration

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

const (
	defaultAmHost      = "localhost"
	defaultAmPort      = "9093"
	defaultAmApiv2path = "/api/v2"
)

// NewAlertmanagerClient initializes an alertmanager client with the given URL
func NewAlertmanagerClient(amURL *url.URL) *client.Alertmanager {
	address := defaultAmHost + ":" + defaultAmPort
	schemes := []string{"http"}

	if amURL.Host != "" {
		address = amURL.Host // URL documents host as host or host:port
	}
	if amURL.Scheme != "" {
		schemes = []string{amURL.Scheme}
	}

	cr := clientruntime.New(address, path.Join(amURL.Path, defaultAmApiv2path), schemes)

	if amURL.User != nil {
		password, _ := amURL.User.Password()
		cr.DefaultAuthentication = clientruntime.BasicAuth(amURL.User.Username(), password)
	}

	return client.New(cr, strfmt.Default)
}

// Execute is the main function for the amtool command
func Execute() {
	var (
		app = kingpin.New("amtool", helpRoot)
	)

	format.InitFormatFlags(app)

	app.Flag("verbose", "Verbose running information").Short('v').BoolVar(&verbose)
	app.Flag("alertmanager.url", "Alertmanager to talk to").URLVar(&alertmanagerURL)
	app.Flag("output", "Output formatter (simple, extended, json)").Short('o').Default("simple").EnumVar(&output, "simple", "extended", "json")
	app.Flag("timeout", "Timeout for the executed command").Default("30s").DurationVar(&timeout)

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
	configureClusterCmd(app)
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
