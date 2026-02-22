package ocpi

import (
	"testing"
	"time"

	"github.com/jaouadou/ocpi-tariff-module/internal/periods"
	"github.com/stretchr/testify/require"
)

func TestCDR_FinalizeAfterWatermark(t *testing.T) {
	t.Parallel()

	sessionID := "session-1"
	startUTC := time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC)
	endUTC := startUTC.Add(30 * time.Minute)

	f := NewFinalizer()

	initialPeriods := []periods.ChargingPeriod{
		{
			Start: startUTC,
			Dimensions: []periods.Dimension{
				{Type: periods.DimensionTypeEnergy, Volume: 1.0},
			},
		},
	}

	updatedPeriods := []periods.ChargingPeriod{
		{
			Start: startUTC,
			Dimensions: []periods.Dimension{
				{Type: periods.DimensionTypeEnergy, Volume: 1.6},
			},
		},
	}

	postSealPeriods := []periods.ChargingPeriod{
		{
			Start: startUTC,
			Dimensions: []periods.Dimension{
				{Type: periods.DimensionTypeEnergy, Volume: 9.9},
			},
		},
	}

	now1 := endUTC.Add(-3 * time.Minute)
	cdr, finalized := f.TryFinalize(sessionID, startUTC, endUTC, endUTC.Add(-1*time.Second), initialPeriods, now1)
	require.False(t, finalized)
	require.Equal(t, CDR{}, cdr)

	now2 := endUTC.Add(-2 * time.Minute)
	cdr, finalized = f.TryFinalize(sessionID, startUTC, endUTC, endUTC.Add(-500*time.Millisecond), updatedPeriods, now2)
	require.False(t, finalized)
	require.Equal(t, CDR{}, cdr)

	now3 := endUTC.Add(1 * time.Minute)
	sealed, finalized := f.TryFinalize(sessionID, startUTC, endUTC, endUTC.Add(1*time.Second), updatedPeriods, now3)
	require.True(t, finalized)
	require.Equal(t, sessionID, sealed.SessionID)
	require.Equal(t, startUTC, sealed.Start)
	require.Equal(t, endUTC, sealed.End)
	require.Equal(t, now3, sealed.FinalizedAt)
	require.Equal(t, updatedPeriods, sealed.ChargingPeriods)

	now4 := endUTC.Add(3 * time.Minute)
	again, finalized := f.TryFinalize(sessionID, startUTC, endUTC, endUTC.Add(10*time.Second), postSealPeriods, now4)
	require.True(t, finalized)
	require.Equal(t, sealed, again)
	require.Equal(t, updatedPeriods, again.ChargingPeriods)
	require.Equal(t, now3, again.FinalizedAt)
}
