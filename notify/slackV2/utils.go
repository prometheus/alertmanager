package slackV2

import "github.com/prometheus/alertmanager/template"

func UniqStr(input []string) []string {
	u := make([]string, 0, len(input))
	m := make(map[string]bool)

	for _, val := range input {
		if _, ok := m[val]; !ok {
			m[val] = true
			u = append(u, val)
		}
	}
	return u
}

func getMapValue(data template.KV, key string) string {
	if value, ok := data[key]; ok {
		return value
	} else {
		return ""
	}
}
