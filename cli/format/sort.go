package format

import (
	"github.com/prometheus/alertmanager/client"
	"github.com/prometheus/alertmanager/types"
)

type ByEndAt []types.Silence

func (s ByEndAt) Len() int           { return len(s) }
func (s ByEndAt) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s ByEndAt) Less(i, j int) bool { return s[i].EndsAt.Before(s[j].EndsAt) }

type ByStartsAt []*client.ExtendedAlert

func (s ByStartsAt) Len() int           { return len(s) }
func (s ByStartsAt) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s ByStartsAt) Less(i, j int) bool { return s[i].StartsAt.Before(s[j].StartsAt) }
