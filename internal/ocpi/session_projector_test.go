package ocpi

import (
	"context"
	"testing"
	"time"

	"github.com/jaouadou/ocpi-tariff-module/internal/periods"
	"github.com/stretchr/testify/require"
)

func TestOCPI_SessionPUT_Idempotent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	session := Session{
		ID: "session-1",
		ChargingPeriods: []periods.ChargingPeriod{
			{
				Start: now.Add(-10 * time.Minute),
				Dimensions: []periods.Dimension{
					{Type: periods.DimensionTypeEnergy, Volume: 1.5},
				},
			},
		},
		LastUpdated: now,
	}

	receiver := NewMockSessionReceiver()
	projector := NewSessionProjector(receiver)

	err := projector.Emit(context.Background(), session)
	require.NoError(t, err)

	err = projector.Emit(context.Background(), session)
	require.NoError(t, err)

	stored, ok := receiver.GetSession(session.ID)
	require.True(t, ok)
	require.Len(t, stored.ChargingPeriods, 1)
	require.Equal(t, session, stored)
}
