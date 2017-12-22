package cli

import (
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
var (
	silenceCmd   = app.Command("silence", "Add, expire or view silences. For more information and additional flags see query help")
	silenceQuiet = silenceCmd.Flag("quiet", "Only show silence ids").Short('q').Bool()
)
