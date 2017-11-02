package format

import (
	"encoding/json"
	"io"
	"os"

	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/types"
)

type JSONFormatter struct {
	writer io.Writer
}

func init() {
	Formatters["json"] = &JSONFormatter{writer: os.Stdout}
}

func (formatter *JSONFormatter) SetOutput(writer io.Writer) {
	formatter.writer = writer
}

func (formatter *JSONFormatter) FormatSilences(silences []types.Silence) error {
	enc := json.NewEncoder(formatter.writer)
	return enc.Encode(silences)
}

func (formatter *JSONFormatter) FormatAlerts(alerts []*dispatch.APIAlert) error {
	enc := json.NewEncoder(formatter.writer)
	return enc.Encode(alerts)
}

func (formatter *JSONFormatter) FormatConfig(config Config) error {
	enc := json.NewEncoder(formatter.writer)
	return enc.Encode(config)
}
