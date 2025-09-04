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
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/alecthomas/kingpin/v2"
	clientruntime "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	promconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/version"
	"golang.org/x/mod/semver"

	"github.com/prometheus/alertmanager/api/v2/client"
	"github.com/prometheus/alertmanager/cli/config"
	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/matcher/compat"
)

var (
	verbose         bool
	alertmanagerURL *url.URL
	output          string
	timeout         time.Duration
	httpConfigFile  string
	versionCheck    bool
	featureFlags    string

	configFiles = []string{os.ExpandEnv("$HOME/.config/amtool/config.yml"), "/etc/amtool/config.yml"}
	legacyFlags = map[string]string{"comment_required": "require-comment"}
)

func initMatchersCompat(_ *kingpin.ParseContext) error {
	promslogConfig := &promslog.Config{Writer: os.Stdout}
	if verbose {
		promslogConfig.Level = promslog.NewLevel()
		_ = promslogConfig.Level.Set("debug")
	}
	logger := promslog.New(promslogConfig)
	featureConfig, err := featurecontrol.NewFlags(logger, featureFlags)
	if err != nil {
		kingpin.Fatalf("error parsing the feature flag list: %v\n", err)
	}
	compat.InitFromFlags(logger, featureConfig)
	return nil
}

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

// NewAlertmanagerClient initializes an alertmanager client with the given URL.
func NewAlertmanagerClient(amURL *url.URL) *client.AlertmanagerAPI {
	address := defaultAmHost + ":" + defaultAmPort
	schemes := []string{"http"}

	if amURL.Host != "" {
		address = amURL.Host // URL documents host as host or host:port
	}
	if amURL.Scheme != "" {
		schemes = []string{amURL.Scheme}
	}

	cr := clientruntime.New(address, path.Join(amURL.Path, defaultAmApiv2path), schemes)

	if amURL.User != nil && httpConfigFile != "" {
		kingpin.Fatalf("basic authentication and http.config.file are mutually exclusive")
	}

	if amURL.User != nil {
		password, _ := amURL.User.Password()
		cr.DefaultAuthentication = clientruntime.BasicAuth(amURL.User.Username(), password)
	}

	if httpConfigFile != "" {
		var err error
		httpConfig, _, err := promconfig.LoadHTTPConfigFile(httpConfigFile)
		if err != nil {
			kingpin.Fatalf("failed to load HTTP config file: %v", err)
		}

		httpclient, err := promconfig.NewClientFromConfig(*httpConfig, "amtool")
		if err != nil {
			kingpin.Fatalf("failed to create a new HTTP client: %v", err)
		}
		cr = clientruntime.NewWithClient(address, path.Join(amURL.Path, defaultAmApiv2path), schemes, httpclient)
	}

	c := client.New(cr, strfmt.Default)

	if !versionCheck {
		return c
	}

	status, err := c.General.GetStatus(nil)
	if err != nil || status.Payload.VersionInfo == nil || version.Version == "" {
		// We can not get version info, or we do not know our own version. Let amtool continue.
		return c
	}

	if semver.MajorMinor("v"+*status.Payload.VersionInfo.Version) != semver.MajorMinor("v"+version.Version) {
		fmt.Fprintf(os.Stderr, "Warning: amtool version (%s) and alertmanager version (%s) are different.\n", version.Version, *status.Payload.VersionInfo.Version)
	}

	return c
}

// Execute is the main function for the amtool command.
func Execute() {
	app := kingpin.New("amtool", helpRoot).UsageWriter(os.Stdout)

	format.InitFormatFlags(app)

	app.Flag("verbose", "Verbose running information").Short('v').BoolVar(&verbose)
	app.Flag("alertmanager.url", "Alertmanager to talk to").URLVar(&alertmanagerURL)
	app.Flag("output", "Output formatter (simple, extended, json)").Short('o').Default("simple").EnumVar(&output, "simple", "extended", "json")
	app.Flag("timeout", "Timeout for the executed command").Default("30s").DurationVar(&timeout)
	app.Flag("http.config.file", "HTTP client configuration file for amtool to connect to Alertmanager.").PlaceHolder("<filename>").ExistingFileVar(&httpConfigFile)
	app.Flag("version-check", "Check alertmanager version. Use --no-version-check to disable.").Default("true").BoolVar(&versionCheck)
	app.Flag("enable-feature", fmt.Sprintf("Experimental features to enable, comma separated. Valid options: %s", strings.Join(featurecontrol.AllowedFlags, ", "))).Default("").StringVar(&featureFlags)

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
	configureTemplateCmd(app)

	app.Action(initMatchersCompat)

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

	http.config.file
		HTTP client configuration file for amtool to connect to Alertmanager.
		The format is https://prometheus.io/docs/alerting/latest/configuration/#http_config.
`
)
