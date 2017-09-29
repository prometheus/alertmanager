package cli

import (
	"fmt"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	"github.com/spf13/cobra"
)

// alertCmd represents the alert command
var checkConfigCmd = &cobra.Command{
	Use:   "check-config file1 [file2] ...",
	Args:  cobra.MinimumNArgs(1),
	Short: "Validate alertmanager config files",
	Long: `Validate alertmanager config files

Will validate the syntax and schema for alertmanager config file
and associated templates. Non existing templates will not trigger
errors`,
	RunE: checkConfig,
}

func init() {
	RootCmd.AddCommand(checkConfigCmd)
	checkConfigCmd.Flags()
}

func checkConfig(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	return CheckConfig(args)
}

func CheckConfig(args []string) error {
	failed := 0

	for _, arg := range args {
		fmt.Printf("Checking '%s'", arg)
		config, _, err := config.LoadFile(arg)
		if err != nil {
			fmt.Printf("  FAILED: %s\n", err)
			failed += 1
		} else {
			fmt.Printf("  SUCCESS\n")
		}

		if config != nil {
			fmt.Printf("Found %d templates: ", len(config.Templates))
			if len(config.Templates) > 0 {
				_, err = template.FromGlobs(config.Templates...)
				if err != nil {
					fmt.Printf("  FAILED: %s\n", err)
					failed += 1
				} else {
					fmt.Printf("  SUCCESS\n")
				}
			}
		}
		fmt.Printf("\n")
	}
	if failed > 0 {
		return fmt.Errorf("Failed to validate %d file(s).", failed)
	}
	return nil
}
