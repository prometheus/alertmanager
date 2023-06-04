package slack

import (
	"encoding/json"
)

type SlackBlock struct {
	Context *SlackBlockContext
	Divider *SlackBlockDivider
	Header  *SlackBlockHeader
	Image   *SlackBlockImage
	Section *SlackBlockSection
}

func (b *SlackBlock) MarshalJSON() ([]byte, error) {
	var v any
	if b.Context != nil {
		v = b.Context
	} else if b.Divider != nil {
		v = b.Divider
	} else if b.Header != nil {
		v = b.Header
	} else if b.Image != nil {
		v = b.Image
	} else {
		v = b.Section
	}
	return json.Marshal(v)
}

type SlackBlockContext struct {
	Elements []SlackBlockContextElement
}

func (b *SlackBlockContext) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type     string                     `json:"type"`
		Elements []SlackBlockContextElement `json:"elements"`
	}{
		Type:     "context",
		Elements: b.Elements,
	})
}

type SlackBlockContextElement struct {
	Image *SlackBlockImage
	Text  *SlackBlockText
}

func (b *SlackBlockContextElement) MarshalJSON() ([]byte, error) {
	var v any
	if b.Image != nil {
		v = b.Image
	} else {
		v = b.Text
	}
	return json.Marshal(v)
}

type SlackBlockDivider struct{}

func (b *SlackBlockDivider) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
	}{
		Type: "divider",
	})
}

type SlackBlockHeader struct {
	Text *SlackBlockText `json:"-"`
}

func (b *SlackBlockHeader) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string          `json:"type"`
		Text *SlackBlockText `json:"text"`
	}{
		Type: "header",
		Text: b.Text,
	})
}

type SlackBlockImage struct {
	ImageURL string
	AltText  string
	Title    string
}

func (b *SlackBlockImage) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type     string `json:"type"`
		ImageURL string `json:"image_url"`
		AltText  string `json:"alt_text"`
		Title    string `json:"title,omitempty"`
	}{
		Type:     "image",
		ImageURL: b.ImageURL,
		AltText:  b.AltText,
		Title:    b.Title,
	})
}

type SlackBlockSection struct {
	Text   *SlackBlockText
	Fields []*SlackBlockText
}

func (b *SlackBlockSection) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type   string            `json:"type"`
		Text   *SlackBlockText   `json:"text,omitempty"`
		Fields []*SlackBlockText `json:"fields,omitempty"`
	}{
		Type:   "section",
		Text:   b.Text,
		Fields: b.Fields,
	})
}

type SlackBlockText struct {
	Text     string
	Emoji    bool
	Markdown bool
}

func (b *SlackBlockText) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type  string `json:"type"`
		Text  string `json:"text"`
		Emoji bool   `json:"emoji,omitempty"`
	}{
		Type: func() string {
			if b.Markdown {
				return "mrkdwn"
			} else {
				return "plain_text"
			}
		}(),
		Text:  b.Text,
		Emoji: b.Emoji,
	})
}
