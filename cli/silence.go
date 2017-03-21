package cli

import (
	"github.com/spf13/cobra"

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
	RunE:  query,
}

func init() {
	RootCmd.AddCommand(silenceCmd)
	silenceCmd.AddCommand(addCmd)
	silenceCmd.AddCommand(expireCmd)
	silenceCmd.AddCommand(queryCmd)
}
