package manager

import (
	"testing"

	"github.com/prometheus/common/mode"
)

func TestAggrGroupInsert(t *testing.T) {
	ag := newAggrGroup(nil, model.LabelSet{
		model.AlertNameLabel: "test",
	}, opts)
}
