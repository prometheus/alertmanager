package format

import (
	"encoding/json"
	"io"

	"github.com/prometheus/alertmanager/types"
)

type JsonFormatter struct {
	writer io.Writer
}

func init() {
	Formatters["json"] = &JsonFormatter{}
}

func (formatter *JsonFormatter) Init(writer io.Writer) {
	formatter.writer = writer
}

func (formatter *JsonFormatter) Format(silences []types.Silence) error {
	enc := json.NewEncoder(formatter.writer)
	return enc.Encode(silences)
}
