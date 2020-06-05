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

package v2

import (
	"strconv"
	"testing"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	open_api_models "github.com/prometheus/alertmanager/api/v2/models"
	general_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/general"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
)

// If api.peers == nil, Alertmanager cluster feature is disabled. Make sure to
// not try to access properties of peer, which would trigger a nil pointer
// dereference.
func TestGetStatusHandlerWithNilPeer(t *testing.T) {
	api := API{
		uptime:             time.Now(),
		peer:               nil,
		alertmanagerConfig: &config.Config{},
	}

	// Test ensures this method call does not panic.
	status := api.getStatusHandler(general_ops.GetStatusParams{}).(*general_ops.GetStatusOK)

	c := status.Payload.Cluster

	if c == nil || c.Status == nil {
		t.Fatal("expected cluster status not to be nil, violating the openapi specification")
	}

	if c.Peers == nil {
		t.Fatal("expected cluster peers to be not nil when api.peer is nil, violating the openapi specification")
	}
	if len(c.Peers) != 0 {
		t.Fatal("expected cluster peers to be empty when api.peer is nil, violating the openapi specification")
	}

	if c.Name != "" {
		t.Fatal("expected cluster name to be empty, violating the openapi specification")
	}
}

func assertEqualStrings(t *testing.T, expected string, actual string) {
	if expected != actual {
		t.Fatal("expected: ", expected, ", actual: ", actual)
	}
}

var (
	testComment = "comment"
	createdBy   = "test"
)

func gettableSilence(id string, state string,
	updatedAt string, start string, end string,
) *open_api_models.GettableSilence {

	updAt, err := strfmt.ParseDateTime(updatedAt)
	if err != nil {
		panic(err)
	}
	strAt, err := strfmt.ParseDateTime(start)
	if err != nil {
		panic(err)
	}
	endAt, err := strfmt.ParseDateTime(end)
	if err != nil {
		panic(err)
	}
	return &open_api_models.GettableSilence{
		Silence: open_api_models.Silence{
			StartsAt:  &strAt,
			EndsAt:    &endAt,
			Comment:   &testComment,
			CreatedBy: &createdBy,
		},
		ID:        &id,
		UpdatedAt: &updAt,
		Status: &open_api_models.SilenceStatus{
			State: &state,
		},
	}
}

func TestGetSilencesHandler(t *testing.T) {

	updateTime := "2019-01-01T12:00:00+00:00"
	silences := []*open_api_models.GettableSilence{
		gettableSilence("silence-6-expired", "expired", updateTime,
			"2019-01-01T12:00:00+00:00", "2019-01-01T11:00:00+00:00"),
		gettableSilence("silence-1-active", "active", updateTime,
			"2019-01-01T12:00:00+00:00", "2019-01-01T13:00:00+00:00"),
		gettableSilence("silence-7-expired", "expired", updateTime,
			"2019-01-01T12:00:00+00:00", "2019-01-01T10:00:00+00:00"),
		gettableSilence("silence-5-expired", "expired", updateTime,
			"2019-01-01T12:00:00+00:00", "2019-01-01T12:00:00+00:00"),
		gettableSilence("silence-0-active", "active", updateTime,
			"2019-01-01T12:00:00+00:00", "2019-01-01T12:00:00+00:00"),
		gettableSilence("silence-4-pending", "pending", updateTime,
			"2019-01-01T13:00:00+00:00", "2019-01-01T12:00:00+00:00"),
		gettableSilence("silence-3-pending", "pending", updateTime,
			"2019-01-01T12:00:00+00:00", "2019-01-01T12:00:00+00:00"),
		gettableSilence("silence-2-active", "active", updateTime,
			"2019-01-01T12:00:00+00:00", "2019-01-01T14:00:00+00:00"),
	}
	sortSilences(open_api_models.GettableSilences(silences))

	for i, sil := range silences {
		assertEqualStrings(t, "silence-"+strconv.Itoa(i)+"-"+*sil.Status.State, *sil.ID)
	}
}

func convertDateTime(ts time.Time) *strfmt.DateTime {
	dt := strfmt.DateTime(ts)
	return &dt
}

func TestAlertToOpenAPIAlert(t *testing.T) {
	var (
		start     = time.Now().Add(-time.Minute)
		updated   = time.Now()
		active    = "active"
		fp        = "0223b772b51c29e1"
		receivers = []string{"receiver1", "receiver2"}

		alert = &types.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"severity": "critical", "alertname": "alert1"},
				StartsAt: start,
			},
			UpdatedAt: updated,
		}
	)
	openAPIAlert := alertToOpenAPIAlert(alert, types.AlertStatus{State: types.AlertStateActive}, receivers)
	require.Equal(t, &open_api_models.GettableAlert{
		Annotations: open_api_models.LabelSet{},
		Alert: open_api_models.Alert{
			Labels: open_api_models.LabelSet{"severity": "critical", "alertname": "alert1"},
		},
		Status: &open_api_models.AlertStatus{
			State:       &active,
			InhibitedBy: []string{},
			SilencedBy:  []string{},
		},
		StartsAt:    convertDateTime(start),
		EndsAt:      convertDateTime(time.Time{}),
		UpdatedAt:   convertDateTime(updated),
		Fingerprint: &fp,
		Receivers: []*open_api_models.Receiver{
			&open_api_models.Receiver{Name: &receivers[0]},
			&open_api_models.Receiver{Name: &receivers[1]},
		},
	}, openAPIAlert)
}
