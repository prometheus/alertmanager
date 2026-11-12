// Copyright The Prometheus Authors
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

package silence

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

func checkErr(t *testing.T, expected string, got error) {
	t.Helper()

	if expected == "" {
		require.NoError(t, got)
		return
	}

	if got == nil {
		t.Errorf("expected error containing %q but got none", expected)
		return
	}

	require.Contains(t, got.Error(), expected)
}

// requireStatesEqual compares two silence states using proto.Equal for proper protobuf comparison.
func requireStatesEqual(t *testing.T, expected, actual state, msgAndArgs ...any) {
	t.Helper()
	require.Len(t, actual, len(expected), msgAndArgs...)
	for id, expectedSil := range expected {
		actualSil, ok := actual[id]
		require.True(t, ok, "silence %s missing from actual state", id)
		require.True(t, proto.Equal(expectedSil, actualSil), "silence %s mismatch: expected %v, got %v", id, expectedSil, actualSil)
	}
}

func TestOptionsValidate(t *testing.T) {
	cases := []struct {
		options *Options
		err     string
	}{
		{
			options: &Options{
				Metrics:        prometheus.NewRegistry(),
				SnapshotReader: &bytes.Buffer{},
			},
		},
		{
			options: &Options{
				Metrics:      prometheus.NewRegistry(),
				SnapshotFile: "test.bkp",
			},
		},
		{
			options: &Options{
				Metrics:        prometheus.NewRegistry(),
				SnapshotFile:   "test bkp",
				SnapshotReader: &bytes.Buffer{},
			},
			err: "only one of SnapshotFile and SnapshotReader must be set",
		},
	}

	for _, c := range cases {
		checkErr(t, c.err, c.options.validate())
	}
}

func TestLogSilence(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	s := &Silences{
		logger: logger,
	}

	now := s.nowUTC()

	silence := &pb.Silence{
		Id:        "test-silence-id",
		CreatedBy: "test-user",
		Comment:   "Test silence comment",
		StartsAt:  timestamppb.New(now),
		EndsAt:    timestamppb.New(now.Add(time.Hour)),
		MatcherSets: []*pb.MatcherSet{
			{Matchers: []*pb.Matcher{
				{Name: "job", Type: pb.Matcher_EQUAL, Pattern: "test-job"},
				{Name: "instance", Type: pb.Matcher_REGEXP, Pattern: ".*example.*"},
				{Name: "severity", Type: pb.Matcher_NOT_EQUAL, Pattern: "info"},
				{Name: "alertname", Type: pb.Matcher_NOT_REGEXP, Pattern: "Test.*"},
			}},
		},
	}

	s.logSilence("test message", silence)
	logOutput := buf.String()

	require.Contains(t, logOutput, "msg=\"test message\"")
	require.Contains(t, logOutput, "Id=test-silence-id")
	require.Contains(t, logOutput, "CreatedBy=test-user")
	require.Contains(t, logOutput, "Comment=\"Test silence comment\"")
	require.Contains(t, logOutput, fmt.Sprintf("StartsAt=%s", now.Format(time.RFC3339)))
	require.Contains(t, logOutput, fmt.Sprintf("EndsAt=%s", now.Add(time.Hour).Format(time.RFC3339)))
	require.Contains(t, logOutput, "Matchers=\"job=test-job,instance=~.*example.*,severity!=info,alertname!~Test.*\"")
}

func TestSilenceAnnotations(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, err := New(Options{
			Metrics:   prometheus.NewRegistry(),
			Retention: time.Hour,
		})
		require.NoError(t, err)

		now := s.nowUTC()

		// Create a silence with annotations
		sil1 := &pb.Silence{
			Matchers: []*pb.Matcher{{Name: "job", Pattern: "test"}},
			StartsAt: timestamppb.New(now),
			EndsAt:   timestamppb.New(now.Add(time.Hour)),
			Annotations: map[string]string{
				"ticket": "JIRA-123",
				"type":   "planned",
				"test":   "integration",
			},
		}

		// Set the silence via the API
		require.NoError(t, s.Set(t.Context(), sil1))
		require.NotEmpty(t, sil1.Id)

		// Query the silence back by ID
		queriedSil, err := s.QueryOne(t.Context(), QIDs(sil1.Id))
		require.NoError(t, err)

		// Verify all annotations are returned correctly
		require.NotNil(t, queriedSil.Annotations)
		require.Equal(t, "JIRA-123", queriedSil.Annotations["ticket"])
		require.Equal(t, "planned", queriedSil.Annotations["type"])
		require.Equal(t, "integration", queriedSil.Annotations["test"])

		// Test querying all silences
		allSils, _, err := s.Query(t.Context())
		require.NoError(t, err)
		require.Len(t, allSils, 1)
		require.Equal(t, queriedSil.Annotations, allSils[0].Annotations)

		// Create a second silence with different annotations
		sil2 := &pb.Silence{
			Matchers: []*pb.Matcher{{Name: "job", Pattern: "frontend"}},
			StartsAt: timestamppb.New(now),
			EndsAt:   timestamppb.New(now.Add(time.Hour)),
			Annotations: map[string]string{
				"ticket": "JIRA-456",
			},
		}
		require.NoError(t, s.Set(t.Context(), sil2))

		// Query by state and verify both silences have their annotations
		activeSils, _, err := s.Query(t.Context(), QState(SilenceStateActive))
		require.NoError(t, err)
		require.Len(t, activeSils, 2)

		for _, sil := range activeSils {
			require.NotNil(t, sil.Annotations)
			switch sil.Id {
			case sil1.Id:
				require.Len(t, sil.Annotations, 3)
				require.Equal(t, "JIRA-123", sil.Annotations["ticket"])
			case sil2.Id:
				require.Len(t, sil.Annotations, 1)
				require.Equal(t, "JIRA-456", sil.Annotations["ticket"])
			default:
				t.Fatalf("unexpected silence ID: %s", sil.Id)
			}
		}

		// Test updating a silence with new annotations
		time.Sleep(time.Minute)
		sil1Updated := &pb.Silence{
			Id:       sil1.Id,
			Matchers: []*pb.Matcher{{Name: "job", Pattern: "test"}},
			StartsAt: sil1.StartsAt,
			EndsAt:   sil1.EndsAt,
			Annotations: map[string]string{
				"ticket": "JIRA-123",
				"type":   "emergency", // changed
				"test":   "load",      // changed
			},
		}
		require.NoError(t, s.Set(t.Context(), sil1Updated))

		// Query back and verify annotations were updated
		queriedUpdated, err := s.QueryOne(t.Context(), QIDs(sil1.Id))
		require.NoError(t, err)
		require.Len(t, queriedUpdated.Annotations, 3)
		require.Equal(t, "emergency", queriedUpdated.Annotations["type"])
		require.Equal(t, "load", queriedUpdated.Annotations["test"])

		// Test silence with nil annotations
		sil3 := &pb.Silence{
			Matchers:    []*pb.Matcher{{Name: "job", Pattern: "backend"}},
			StartsAt:    timestamppb.New(now),
			EndsAt:      timestamppb.New(now.Add(time.Hour)),
			Annotations: nil,
		}
		require.NoError(t, s.Set(t.Context(), sil3))
		queriedSil3, err := s.QueryOne(t.Context(), QIDs(sil3.Id))
		require.NoError(t, err)
		// nil annotations should be preserved or converted to empty map
		if queriedSil3.Annotations != nil {
			require.Empty(t, queriedSil3.Annotations)
		}

		// Test silence with empty annotations map
		sil4 := &pb.Silence{
			Matchers:    []*pb.Matcher{{Name: "job", Pattern: "database"}},
			StartsAt:    timestamppb.New(now),
			EndsAt:      timestamppb.New(now.Add(time.Hour)),
			Annotations: map[string]string{},
		}
		require.NoError(t, s.Set(t.Context(), sil4))
		queriedSil4, err := s.QueryOne(t.Context(), QIDs(sil4.Id))
		require.NoError(t, err)
		if queriedSil4.Annotations != nil {
			require.Empty(t, queriedSil4.Annotations)
		}
	})
}
