// Copyright 2019 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package slack

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
)

// handleThreadedSummaryHeaderMode implements message_strategy "thread" when
// use_summary_header is true: the first post is a compact parent summary (title/color
// from transition state); each notify appends a threaded reply and refreshes the parent.
// TmplErr points to the template error variable updated by tmplText.
func (n *Notifier) handleThreadedSummaryHeaderMode(ctx context.Context, data *template.Data, tmplText func(string) string, tmplErr *error, store *nflog.Store, u string, req *request, logger *slog.Logger) (bool, error) {
	content, err := n.buildThreadSummaryHeaderContent(ctx, store, data, tmplText, tmplErr)
	if err != nil {
		return false, err
	}

	parentThreadTs, parentChannelId, parentFound, retry, err := n.ensureSummaryHeaderParent(ctx, u, store, tmplText, tmplErr, content)
	if err != nil {
		return retry, err
	}

	if parentThreadTs == "" || parentChannelId == "" {
		return false, fmt.Errorf("cannot post slack thread reply: missing parent thread ts or channel id")
	}

	req.ThreadTimestamp = parentThreadTs
	req.Channel = parentChannelId
	retry, err = n.postAndHandle(ctx, u, req.Channel, req, nil, slackResponseOpts{})
	if err != nil {
		return retry, err
	}

	n.completeSummaryHeaderNotification(ctx, parentFound, parentChannelId, parentThreadTs, content, store, logger)
	return false, nil
}

// handleThreadedDirectMode implements message_strategy "thread" when use_summary_header
// is false: the first notification is the thread parent; later notifications post as
// replies (thread_ts). Parent ts/channel are stored only on the initial post.
func (n *Notifier) handleThreadedDirectMode(ctx context.Context, store *nflog.Store, req *request, u string, logger *slog.Logger) (bool, error) {
	parentThreadTs, parentChannelId, parentFound := getStoredParent(store)
	if parentFound {
		req.ThreadTimestamp = parentThreadTs
		req.Channel = parentChannelId
	}

	storeForPost := store
	if parentFound {
		storeForPost = nil
	}
	retry, err := n.postAndHandle(ctx, u, req.Channel, req, storeForPost, slackResponseOpts{})
	if err != nil {
		return retry, err
	}

	if !parentFound {
		parentThreadTs, parentChannelId, _ = getStoredParent(store)
	}

	reason, reasonOk := notify.NotificationReason(ctx)
	if reasonOk && reason == notify.ReasonAllAlertsResolved && n.conf.ThreadedOptions != nil {
		if n.conf.ThreadedOptions.ResolveEmoji != "" {
			if err := n.addResolveEmoji(ctx, parentChannelId, parentThreadTs, n.conf.ThreadedOptions.ResolveEmoji); err != nil {
				logger.Warn("failed to add resolve emoji reaction", "err", err)
			} else {
				logger.Debug("added resolve emoji reaction to thread", "emoji", n.conf.ThreadedOptions.ResolveEmoji, storeKeyThreadTs, parentThreadTs)
			}
		}
	}

	return false, nil
}

// ensureSummaryHeaderParent returns thread ts and channel for the summary parent,
// Posting the parent message when missing. parentFound is true when ids were
// already in nflog before this call.
func (n *Notifier) ensureSummaryHeaderParent(
	ctx context.Context,
	u string,
	store *nflog.Store,
	tmplText func(string) string,
	tmplErr *error,
	content threadSummaryHeaderContent,
) (parentThreadTs, parentChannelId string, parentFound bool, retry bool, err error) {
	parentThreadTs, parentChannelId, parentFound = getStoredParent(store)

	if !parentFound {
		parentReq := &request{
			Channel:   tmplText(n.conf.Channel),
			Username:  tmplText(n.conf.Username),
			IconEmoji: tmplText(n.conf.IconEmoji),
			IconURL:   tmplText(n.conf.IconURL),
			LinkNames: n.conf.LinkNames,
			Attachments: []attachment{{
				Title: content.title,
				Color: content.color,
			}},
		}
		if *tmplErr != nil {
			return "", "", false, false, *tmplErr
		}
		retry, err := n.postAndHandle(ctx, u, parentReq.Channel, parentReq, store, slackResponseOpts{})
		if err != nil {
			return "", "", false, retry, err
		}
		parentThreadTs, parentChannelId, _ = getStoredParent(store)
		if parentThreadTs == "" || parentChannelId == "" {
			return "", "", false, false, fmt.Errorf("slack summary parent post succeeded but nflog has no thread ts or channel id")
		}
	}

	return parentThreadTs, parentChannelId, parentFound, false, nil
}

// buildThreadSummaryHeaderContent derives transition history, summary title, and colors
// From nflog, context, and templates. tmplErr is the shared template error pointer from Notify.
func (n *Notifier) buildThreadSummaryHeaderContent(ctx context.Context, store *nflog.Store, data *template.Data, tmplText func(string) string, tmplErr *error) (threadSummaryHeaderContent, error) {
	previousTransitions, _ := store.GetStr(storeKeyTransitions)
	reason, reasonOk := notify.NotificationReason(ctx)
	label := ""
	if reasonOk {
		label = reasonToTransition(reason)
	} else {
		label = "UNKNOWN"
	}
	transitions := previousTransitions
	if label != "" {
		if transitions != "" {
			transitions += "|"
		}
		transitions += label
	}

	alertName := data.CommonLabels["alertname"]
	summaryTitle, _ := notify.TruncateInRunes(buildTransitionTitle(transitions, alertName), maxTitleLenRunes)
	summaryColor := tmplText(n.conf.Color)
	if reasonOk && reason == notify.ReasonAllAlertsResolved && n.conf.ThreadedOptions != nil &&
		n.conf.ThreadedOptions.SummaryHeader != nil && n.conf.ThreadedOptions.SummaryHeader.ResolveColor != "" {
		summaryColor = tmplText(n.conf.ThreadedOptions.SummaryHeader.ResolveColor)
	}
	if *tmplErr != nil {
		return threadSummaryHeaderContent{}, *tmplErr
	}
	return threadSummaryHeaderContent{
		transitions: transitions,
		title:       summaryTitle,
		color:       summaryColor,
		reason:      reason,
		reasonOk:    reasonOk,
	}, nil
}

// completeSummaryHeaderNotification runs best-effort parent update and resolve emoji,
// then persists transition history to nflog.
func (n *Notifier) completeSummaryHeaderNotification(ctx context.Context, parentExisted bool, channelId, threadTs string, content threadSummaryHeaderContent, store *nflog.Store, logger *slog.Logger) {
	if parentExisted {
		if err := n.updateParentSummary(ctx, channelId, threadTs, content.title, content.color); err != nil {
			logger.Warn("failed to update parent summary", "err", err)
		} else {
			logger.Debug("updated parent summary", storeKeyThreadTs, threadTs, "title", content.title)
		}
	}

	if content.reasonOk && content.reason == notify.ReasonAllAlertsResolved &&
		n.conf.ThreadedOptions != nil && n.conf.ThreadedOptions.ResolveEmoji != "" {
		if err := n.addResolveEmoji(ctx, channelId, threadTs, n.conf.ThreadedOptions.ResolveEmoji); err != nil {
			logger.Warn("failed to add resolve emoji reaction", "err", err)
		} else {
			logger.Debug("added resolve emoji reaction to thread", "emoji", n.conf.ThreadedOptions.ResolveEmoji, storeKeyThreadTs, threadTs)
		}
	}

	store.SetStr(storeKeyTransitions, content.transitions)
}

// addResolveEmoji calls reactions.add on the message identified by channel and timestamp.
// Errors are logged as warnings by callers; already_reacted is treated as success.
func (n *Notifier) addResolveEmoji(ctx context.Context, channel, timestamp, emoji string) error {
	reqBody := reactionRequest{
		Channel:   channel,
		Name:      emoji,
		Timestamp: timestamp,
	}

	reactionsURL, err := n.urlResolver.URLForMethod("reactions.add")
	if err != nil {
		return fmt.Errorf("slack reactions.add url: %w", err)
	}
	_, err = n.postAndHandle(ctx, reactionsURL, channel, reqBody, nil, slackResponseOpts{
		IgnoreAPIErrors: []string{"already_reacted"},
	})
	if err != nil {
		return fmt.Errorf("posting reaction: %w", err)
	}
	return nil
}

// updateParentSummary calls chat.update on the parent message (summary header mode)
// to refresh attachment title and color after a transition. Failures are best-effort
// (callers log warnings).
func (n *Notifier) updateParentSummary(ctx context.Context, channel, timestamp, title, color string) error {
	updateReq := &request{
		Timestamp: timestamp,
		Channel:   channel,
		Attachments: []attachment{{
			Title: title,
			Color: color,
		}},
	}

	updateURL, err := n.urlResolver.URLForMethod("chat.update")
	if err != nil {
		return err
	}
	_, err = n.postAndHandle(ctx, updateURL, channel, updateReq, nil, slackResponseOpts{})
	if err != nil {
		return fmt.Errorf("posting update: %w", err)
	}
	return nil
}

// getStoredParent retrieves the thread parent's timestamp and channel from the nflog store.
// Found is true only when both keys are present, guarding against partial state.
func getStoredParent(store *nflog.Store) (threadTs, channelId string, found bool) {
	threadTs, tsOk := store.GetStr(storeKeyThreadTs)
	channelId, chOk := store.GetStr(storeKeyChannelId)
	return threadTs, channelId, tsOk && chOk
}

// reasonToTransition maps a NotifyReason to a short transition label.
// Returns empty string for reasons that don't represent a state change.
func reasonToTransition(reason notify.NotifyReason) string {
	switch reason {
	case notify.ReasonFirstNotification, notify.ReasonNewAlertsInGroup:
		return "FIRING"
	case notify.ReasonNewResolvedAlerts:
		return "PARTIAL RESOLVE"
	case notify.ReasonAllAlertsResolved:
		return "RESOLVED"
	default:
		return ""
	}
}

// buildTransitionTitle collapses a pipe-delimited transition history into a
// human-readable title with counts for consecutive duplicate states.
// Example: "FIRING|FIRING|PARTIAL RESOLVE|RESOLVED" → "FIRING (2) → PARTIAL RESOLVE → RESOLVED: MyAlert".
func buildTransitionTitle(transitions, alertName string) string {
	parts := strings.Split(transitions, "|")

	type entry struct {
		label string
		count int
	}
	var collapsed []entry
	for _, p := range parts {
		if p == "" {
			continue
		}
		if len(collapsed) > 0 && collapsed[len(collapsed)-1].label == p {
			collapsed[len(collapsed)-1].count++
		} else {
			collapsed = append(collapsed, entry{label: p, count: 1})
		}
	}

	if len(collapsed) == 0 {
		return alertName
	}

	var sb strings.Builder
	for i, e := range collapsed {
		if i > 0 {
			sb.WriteString(" → ")
		}
		sb.WriteString(e.label)
		if e.count > 1 {
			sb.WriteString(" (")
			sb.WriteString(strconv.Itoa(e.count))
			sb.WriteString(")")
		}
	}
	if alertName != "" {
		sb.WriteString(": ")
		sb.WriteString(alertName)
	}
	return sb.String()
}
