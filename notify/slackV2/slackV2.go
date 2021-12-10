package slackV2

import (
	"context"
	"fmt"

	"github.com/go-kit/log"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/slack-go/slack"
)

// Notifier implements a Notifier for Slack notifications.
type Notifier struct {
	conf   *config.SlackConfigV2
	tmpl   *template.Template
	logger log.Logger
	client *slack.Client
}

// New returns a new Slack notification handler.
func New(c *config.SlackConfigV2, t *template.Template, l log.Logger, ) (*Notifier, error) {
	token := c.Token
	client := slack.New(token)

	return &Notifier{
		conf:   c,
		tmpl:   t,
		logger: l,
		client: client,
	}, nil
}

// attachment is used to display a richly-formatted message block.

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var err error
	var (
		data     = notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
		tmplText = notify.TmplText(n.tmpl, data, &err)
	)

	attachmets := &slack.Attachment{
		Title:     n.conf.Title,
		TitleLink: n.conf.TitleLink,
		Pretext:   n.conf.Pretext,
		Text:      tmplText(n.conf.Text),
		ImageURL:  n.conf.ImageURL,
		Footer:    n.conf.Footer,
		Color:     n.conf.Color,
	}

	//params2 := &slack.PostMessageParameters{
	//	Username:    n.conf.Username,
	//	IconEmoji:   n.conf.IconEmoji,
	//}

	params := slack.NewPostMessageParameters()
	params.Username = n.conf.Username
	params.IconEmoji = n.conf.IconEmoji

	txt := slack.MsgOptionText("hello", false)
	att := slack.MsgOptionAttachments(*attachmets)
	//params.Attachments = []slack.Attachment{att}
	channal, ts, err := n.client.PostMessage(n.conf.Channel, txt, att)
	fmt.Printf(channal, ts)

}
