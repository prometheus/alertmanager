package cli

import (
	"fmt"
	"os"

	"github.com/prometheus/alertmanager/cli/format"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "amtool",
	Short: "Alertmanager CLI",
	Long: `View and modify the current Alertmanager state.

[Config File]

The alertmanager tool will read a config file from the --config cli argument, AMTOOL_CONFIG environment variable or
from one of two default config locations. Valid config file formats are JSON, TOML, YAML, HCL and Java Properties, use
whatever makes sense for your project.

The default config file paths are $HOME/.config/amtool/config.yml or /etc/amtool/config.yml

The accepted config options are as follows:

	alertmanager.url
		Set a default alertmanager url for each request

	author
		Set a default author value for new silences. If this argument is not specified then the username will be used

	comment_required
		Require a comment on silence creation

	output
		Set a default output type. Options are (simple, extended, json)
	`,
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().String("config", "", "config file (default is $HOME/.config/amtool/config.yml)")
	viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
	RootCmd.PersistentFlags().String("alertmanager.url", "", "Alertmanager to talk to")
	viper.BindPFlag("alertmanager.url", RootCmd.PersistentFlags().Lookup("alertmanager.url"))
	RootCmd.PersistentFlags().StringP("output", "o", "simple", "Output formatter (simple, extended, json)")
	viper.BindPFlag("output", RootCmd.PersistentFlags().Lookup("output"))
	RootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose running information")
	viper.BindPFlag("verbose", RootCmd.PersistentFlags().Lookup("verbose"))
	viper.SetDefault("date.format", format.DefaultDateFormat)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	viper.SetConfigName("config") // name of config file (without extension)
	viper.AddConfigPath("/etc/amtool")
	viper.AddConfigPath("$HOME/.config/amtool")
	viper.SetEnvPrefix("AMTOOL")
	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	cfgFile := viper.GetString("config")
	if cfgFile != "" { // enable ability to specify config file via flag
		viper.SetConfigFile(cfgFile)
	}
	err := viper.ReadInConfig()
	if err == nil {
		if viper.GetBool("verbose") {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}
}
