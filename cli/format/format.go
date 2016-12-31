package format

import (
	"io"

	"github.com/prometheus/alertmanager/types"
)

type Formatter interface {
	Init(io.Writer)
	Format([]types.Silence) error
}

var Formatters map[string]Formatter = map[string]Formatter{}
