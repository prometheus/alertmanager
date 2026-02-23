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

package marker

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
)

func TestAlertMarker_Status(t *testing.T) {
	gm := NewAlertMarker()

	fp1 := model.Fingerprint(1)
	fp2 := model.Fingerprint(2)

	sil1 := "sil-1"
	sil2 := "sil-2"

	// Untracked alert should return unprocessed.
	status := gm.Status(fp1)
	require.Equal(t, alert.AlertStateUnprocessed, status.State)
	require.Empty(t, status.SilencedBy)
	require.Empty(t, status.InhibitedBy)

	// Silence an alert.
	gm.SetSilenced(fp1, []string{sil1, sil2})
	status = gm.Status(fp1)
	require.Equal(t, alert.AlertStateSuppressed, status.State)
	require.Equal(t, []string{sil1, sil2}, status.SilencedBy)
	require.Empty(t, status.InhibitedBy)

	// A different alert should still be unprocessed.
	status = gm.Status(fp2)
	require.Equal(t, alert.AlertStateUnprocessed, status.State)

	// Different AlertMarker instances are independent.
	gm2 := NewAlertMarker()
	status = gm2.Status(fp1)
	require.Equal(t, alert.AlertStateUnprocessed, status.State)

	// Remove silences → should become active.
	gm.SetSilenced(fp1, nil)
	status = gm.Status(fp1)
	require.Equal(t, alert.AlertStateActive, status.State)
	require.Empty(t, status.SilencedBy)
}

func TestAlertMarker_Inhibited(t *testing.T) {
	gm := NewAlertMarker()

	fp := model.Fingerprint(1)
	sil1 := "sil-1"

	// Inhibit an alert.
	gm.SetInhibited(fp, []string{"src-1"})
	status := gm.Status(fp)
	require.Equal(t, alert.AlertStateSuppressed, status.State)
	require.Equal(t, []string{"src-1"}, status.InhibitedBy)

	// Also silence it — should remain suppressed.
	gm.SetSilenced(fp, []string{sil1})
	status = gm.Status(fp)
	require.Equal(t, alert.AlertStateSuppressed, status.State)
	require.Equal(t, []string{sil1}, status.SilencedBy)
	require.Equal(t, []string{"src-1"}, status.InhibitedBy)

	// Clear inhibition — still suppressed by silence.
	gm.SetInhibited(fp, nil)
	status = gm.Status(fp)
	require.Equal(t, alert.AlertStateSuppressed, status.State)
	require.Empty(t, status.InhibitedBy)
	require.Equal(t, []string{sil1}, status.SilencedBy)

	// Clear silence too — now active.
	gm.SetSilenced(fp, nil)
	status = gm.Status(fp)
	require.Equal(t, alert.AlertStateActive, status.State)
}

func TestAlertMarker_Delete(t *testing.T) {
	gm := NewAlertMarker()

	fp1 := model.Fingerprint(1)
	fp2 := model.Fingerprint(2)

	gm.SetSilenced(fp1, []string{"sil-1"})
	gm.SetSilenced(fp2, []string{"sil-2"})

	// Delete fp1, fp2 should remain.
	gm.Delete(fp1)

	status := gm.Status(fp1)
	require.Equal(t, alert.AlertStateUnprocessed, status.State)

	status = gm.Status(fp2)
	require.Equal(t, alert.AlertStateSuppressed, status.State)
	require.Equal(t, []string{"sil-2"}, status.SilencedBy)
}

func TestAlertMarkerContext(t *testing.T) {
	ctx := t.Context()

	// No marker in context — should return no-op marker.
	got := GetAlertMarker(ctx)
	require.NotNil(t, got)
	// No-op marker returns unknown for any fingerprint.
	require.Equal(t, alert.AlertStateUnprocessed, got.Status(model.Fingerprint(1)).State)

	// Set marker in context.
	gm := NewAlertMarker()
	ctx = WithAlertMarker(ctx, gm)
	got = GetAlertMarker(ctx)
	require.NotNil(t, got)

	// Writing through the context-extracted marker should be visible.
	fp := model.Fingerprint(1)
	got.SetInhibited(fp, []string{"src-1"})
	status := gm.Status(fp)
	require.Equal(t, alert.AlertStateSuppressed, status.State)
}
