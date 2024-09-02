// Copyright 2024 The Prometheus Authors
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

package msteams

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewAdaptiveCard(t *testing.T) {
	card := NewAdaptiveCard()

	require.NotNil(t, card)
	require.Equal(t, "http://adaptivecards.io/schemas/adaptive-card.json", card.Schema)
	require.Equal(t, "AdaptiveCard", card.Type)
	require.Equal(t, "1.4", card.Version)
	require.Empty(t, card.Body)
}

func TestAdaptiveCard_MarshalJSON(t *testing.T) {
	card := NewAdaptiveCard()
	card.AppendItem(AdaptiveCardTextBlockItem{Text: "Text"})

	bytes, err := card.MarshalJSON()
	require.NoError(t, err)

	expectedJSON := `
		{
			"body":[
				{"type":"TextBlock","text":"Text"}
			],
			"msTeams":{"width":"Full"},
			"$schema":"http://adaptivecards.io/schemas/adaptive-card.json",
			"type":"AdaptiveCard",
			"version":"1.4"
	}`
	require.JSONEq(t, expectedJSON, string(bytes))
}

func TestNewAdaptiveCardsMessage(t *testing.T) {
	card := NewAdaptiveCard()
	message := NewAdaptiveCardsMessage(card)

	require.Equal(t, "message", message.Type)
	require.Len(t, message.Attachments, 1)
	require.Equal(t, "application/vnd.microsoft.card.adaptive", message.Attachments[0].ContentType)
	require.Equal(t, card, message.Attachments[0].Content)
}

func TestAdaptiveCardTextBlockItem_MarshalJSON(t *testing.T) {
	item := AdaptiveCardTextBlockItem{
		Text:   "hello world",
		Color:  "test-color",
		Size:   "medium",
		Weight: "bold",
		Wrap:   true,
	}

	bytes, err := item.MarshalJSON()
	require.NoError(t, err)

	expectedJSON := `{
		"type": "TextBlock",
		"text": "hello world",
		"color": "test-color",
		"size": "medium",
		"weight": "bold",
		"wrap": true
	}`
	require.JSONEq(t, expectedJSON, string(bytes))
}

func AdaptiveCardActionSetItemMarshalJSON(t *testing.T) {
	item := AdaptiveCardActionSetItem{
		Actions: []AdaptiveCardActionItem{
			AdaptiveCardOpenURLActionItem{
				Title: "View URL",
				URL:   "https://example.com",
			},
		},
	}

	bytes, err := item.MarshalJSON()
	require.NoError(t, err)

	expectedJSON := `{
		"type":"ActionSet",
		"actions":[
			{
				"type":"Action.OpenUrl",
				"title":"View URL",
				"url":"https://example.com"
			}
		]
	}`
	require.JSONEq(t, expectedJSON, string(bytes))
}

func AdaptiveCardOpenURLActionItemMarshalJSON(t *testing.T) {
	item := AdaptiveCardOpenURLActionItem{
		IconURL: "https://example.com/icon.png",
		Title:   "View URL",
		URL:     "https://example.com",
	}

	bytes, err := item.MarshalJSON()
	require.NoError(t, err)

	expectedJSON := `{
		"type":"Action.OpenUrl",
		"title":"View URL",
		"url":"https://example.com",
		"iconUrl":"https://example.com/icon.png"
	}`
	require.JSONEq(t, expectedJSON, string(bytes))
}
