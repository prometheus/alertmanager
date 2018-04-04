package format

import (
	"io"
	"time"

	"github.com/alecthomas/kingpin"

	"github.com/prometheus/alertmanager/cli"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/types"
)

const DefaultDateFormat = "2006-01-02 15:04:05 MST"

var (
	dateFormat *string
)

func InitFormatFlags(app *kingpin.Application) {
	dateFormat = app.Flag("date.format", "Format of date output").Default(DefaultDateFormat).String()
}

// Formatter needs to be implemented for each new output formatter.
type Formatter interface {
	SetOutput(io.Writer)
	FormatSilences([]types.Silence) error
	FormatAlerts([]*dispatch.APIAlert) error
	FormatConfig(*cli.ServerStatus) error
}

// Formatters is a map of cli argument names to formatter interface object.
var Formatters = map[string]Formatter{}

func FormatDate(input time.Time) string {
	return input.Format(*dateFormat)
}
