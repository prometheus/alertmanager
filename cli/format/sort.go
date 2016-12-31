package format

import (
	"github.com/prometheus/alertmanager/types"
)

type ByEndAt []types.Silence

func (s ByEndAt) Len() int           { return len(s) }
func (s ByEndAt) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s ByEndAt) Less(i, j int) bool { return s[i].EndsAt.Before(s[j].EndsAt) }
