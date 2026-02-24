package silence

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCurrentState(t *testing.T) {
	var (
		pastStartTime = time.Now()
		pastEndTime   = time.Now()

		futureStartTime = time.Now().Add(time.Hour)
		futureEndTime   = time.Now().Add(time.Hour)
	)

	expected := CurrentState(futureStartTime, futureEndTime)
	require.Equal(t, SilenceStatePending, expected)

	expected = CurrentState(pastStartTime, futureEndTime)
	require.Equal(t, SilenceStateActive, expected)

	expected = CurrentState(pastStartTime, pastEndTime)
	require.Equal(t, SilenceStateExpired, expected)
}
