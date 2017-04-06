package format

import (
	"encoding/json"
	"io"
	"os"

	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/types"
)

type JsonFormatter struct {
	writer io.Writer
}

func init() {
	Formatters["json"] = &JsonFormatter{writer: os.Stdout}
}

func (formatter *JsonFormatter) SetOutput(writer io.Writer) {
	formatter.writer = writer
}

func (formatter *JsonFormatter) FormatSilences(silences []types.Silence) error {
	enc := json.NewEncoder(formatter.writer)
	return enc.Encode(silences)
}

func (formatter *JsonFormatter) FormatAlerts(alerts []*dispatch.APIAlert) error {
	enc := json.NewEncoder(formatter.writer)
	return enc.Encode(alerts)
}

func (formatter *JsonFormatter) FormatConfig(config Config) error {
	enc := json.NewEncoder(formatter.writer)
	return enc.Encode(config)
}
