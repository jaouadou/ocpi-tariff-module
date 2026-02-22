package breakpoints

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBreakpoints_EnergyThresholdInterpolation(t *testing.T) {
	startUTC := time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC)
	endUTC := time.Date(2026, 2, 22, 10, 10, 0, 0, time.UTC)

	meter := []MeterSample{
		{At: startUTC, TotalKWh: 0},
		{At: time.Date(2026, 2, 22, 10, 8, 0, 0, time.UTC), TotalKWh: 1.6},
		{At: endUTC, TotalKWh: 2},
	}

	calendar := []time.Time{
		time.Date(2026, 2, 22, 10, 7, 0, 0, time.UTC),
		time.Date(2026, 2, 22, 9, 59, 0, 0, time.UTC),
		time.Date(2026, 2, 22, 10, 10, 0, 0, time.UTC),
	}

	thresholds := []EnergyThreshold{{Kind: "max", KWh: 1}}

	got := Breakpoints(startUTC, endUTC, meter, calendar, thresholds)

	require.Equal(t, []time.Time{
		time.Date(2026, 2, 22, 10, 5, 0, 0, time.UTC),
		time.Date(2026, 2, 22, 10, 7, 0, 0, time.UTC),
		time.Date(2026, 2, 22, 10, 8, 0, 0, time.UTC),
		time.Date(2026, 2, 22, 10, 10, 0, 0, time.UTC),
	}, got)
	requireStrictlyIncreasing(t, got)
}

func TestBreakpoints_SkipsFlatEnergyIntervalsForThresholdCrossings(t *testing.T) {
	startUTC := time.Date(2026, 2, 22, 11, 0, 0, 0, time.UTC)
	endUTC := time.Date(2026, 2, 22, 11, 10, 0, 0, time.UTC)

	meter := []MeterSample{
		{At: startUTC, TotalKWh: 5},
		{At: endUTC, TotalKWh: 5},
	}

	thresholds := []EnergyThreshold{{Kind: "min", KWh: 5}}

	got := Breakpoints(startUTC, endUTC, meter, nil, thresholds)

	require.Equal(t, []time.Time{endUTC}, got)
	requireStrictlyIncreasing(t, got)
}

func requireStrictlyIncreasing(t *testing.T, instants []time.Time) {
	t.Helper()

	for i := 1; i < len(instants); i++ {
		require.True(t, instants[i].After(instants[i-1]), "instants must be strictly increasing")
	}
}
