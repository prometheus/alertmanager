package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/prometheus/alertmanager/types"
)

//var labels []string

type alertmanagerSilenceResponse struct {
	Status    string          `json:"status"`
	Data      []types.Silence `json:"data,omitempty"`
	ErrorType string          `json:"errorType,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// silenceCmd represents the silence command
var silenceCmd = &cobra.Command{
	Use:   "silence",
	Short: "Manage silences",
	Long:  `Add, expire or view silences. For more information and additional flags see query help`,
	Run:   CommandWrapper(query),
}

func init() {
	RootCmd.AddCommand(silenceCmd)
	silenceCmd.PersistentFlags().BoolP("quiet", "q", false, "Only show silence ids")
	viper.BindPFlag("quiet", silenceCmd.PersistentFlags().Lookup("quiet"))
	silenceCmd.AddCommand(addCmd)
	silenceCmd.AddCommand(expireCmd)
	silenceCmd.AddCommand(queryCmd)
}
