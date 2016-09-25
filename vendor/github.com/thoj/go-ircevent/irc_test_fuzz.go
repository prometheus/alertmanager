// +build gofuzz

package irc

func Fuzz(data []byte) int {
	b := bytes.NewBuffer(data)
	event, err := parseToEvent(b.String())
	if err == nil {
		irc := IRC("go-eventirc", "go-eventirc")
		irc.RunCallbacks(event)
		return 1
	}
	return 0
}
