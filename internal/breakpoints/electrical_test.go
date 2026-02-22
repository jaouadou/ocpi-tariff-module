package breakpoints

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTelemetry_MaxPowerBandSwitch(t *testing.T) {
	maxPowerKW := 30.0
	tz := time.FixedZone("UTC+2", 2*60*60)

	samples := []PowerSample{
		{At: time.Date(2026, 2, 22, 12, 0, 0, 0, tz), PowerKW: 20},
		{At: time.Date(2026, 2, 22, 12, 5, 0, 0, tz), PowerKW: 35},
		{At: time.Date(2026, 2, 22, 12, 10, 0, 0, tz), PowerKW: 29},
		{At: time.Date(2026, 2, 22, 12, 15, 0, 0, tz), PowerKW: 29},
	}

	got := PowerRestrictionBreakpoints(samples, nil, &maxPowerKW)

	require.Equal(t, []time.Time{
		time.Date(2026, 2, 22, 10, 5, 0, 0, time.UTC),
		time.Date(2026, 2, 22, 10, 10, 0, 0, time.UTC),
	}, got)
	requireStrictlyIncreasing(t, got)
}

func TestCurrentRestrictionBreakpoints_MinInclusiveMaxExclusive(t *testing.T) {
	minCurrentA := 16.0
	maxCurrentA := 32.0

	samples := []CurrentSample{
		{At: time.Date(2026, 2, 22, 10, 10, 0, 0, time.UTC), CurrentA: 31.9},
		{At: time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC), CurrentA: 15.9},
		{At: time.Date(2026, 2, 22, 10, 20, 0, 0, time.UTC), CurrentA: 32.0},
		{At: time.Date(2026, 2, 22, 10, 5, 0, 0, time.UTC), CurrentA: 16.0},
	}

	got := CurrentRestrictionBreakpoints(samples, &minCurrentA, &maxCurrentA)

	require.Equal(t, []time.Time{
		time.Date(2026, 2, 22, 10, 5, 0, 0, time.UTC),
		time.Date(2026, 2, 22, 10, 20, 0, 0, time.UTC),
	}, got)
	requireStrictlyIncreasing(t, got)
}

func TestPowerRestrictionBreakpoints_UnboundedRangeHasNoCrossings(t *testing.T) {
	samples := []PowerSample{
		{At: time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC), PowerKW: 0},
		{At: time.Date(2026, 2, 22, 10, 5, 0, 0, time.UTC), PowerKW: 50},
		{At: time.Date(2026, 2, 22, 10, 10, 0, 0, time.UTC), PowerKW: 10},
	}

	got := PowerRestrictionBreakpoints(samples, nil, nil)

	require.Empty(t, got)
}
