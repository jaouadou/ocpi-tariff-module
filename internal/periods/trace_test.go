package periods

import (
	"strings"
	"testing"
	"time"

	"github.com/ocpi/ocpi/internal/breakpoints"
	"github.com/ocpi/ocpi/internal/tariffs"
	"github.com/stretchr/testify/require"
)

func TestDebugTrace_TariffSwitch(t *testing.T) {
	startUTC := time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC)
	switchTime := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	endUTC := time.Date(2026, 2, 22, 12, 30, 0, 0, time.UTC)

	peakStart := "12:00"
	peakEnd := "18:00"

	tariff := tariffs.Tariff{
		Elements: []tariffs.TariffElement{
			{
				ID: "offpeak",
				PriceComponents: []tariffs.PriceComponent{
					{Type: tariffs.TariffDimensionTypeEnergy},
					{Type: tariffs.TariffDimensionTypeTime},
				},
				Restrictions: tariffs.TariffRestrictions{
					EndTime: &peakStart,
				},
			},
			{
				ID: "peak",
				PriceComponents: []tariffs.PriceComponent{
					{Type: tariffs.TariffDimensionTypeEnergy},
					{Type: tariffs.TariffDimensionTypeTime},
				},
				Restrictions: tariffs.TariffRestrictions{
					StartTime: &peakStart,
					EndTime:   &peakEnd,
				},
			},
		},
	}

	meter := []breakpoints.MeterSample{
		{At: startUTC, TotalKWh: 0},
		{At: switchTime, TotalKWh: 1.5},
		{At: endUTC, TotalKWh: 3},
	}

	trace := &Trace{}

	periods, err := AccumulateWithTrace(startUTC, endUTC, tariff, meter, nil, nil, nil, nil, trace)
	require.NoError(t, err)
	require.Len(t, periods, 2)

	require.Equal(t, startUTC, periods[0].Start)
	require.Equal(t, switchTime, periods[1].Start)

	require.NotEmpty(t, trace.Events)

	var tariffSwitchEvent *TraceEvent
	for i := range trace.Events {
		if trace.Events[i].Reason == TraceReasonTariffSwitch {
			event := trace.Events[i]
			tariffSwitchEvent = &event
			break
		}
	}

	require.NotNil(t, tariffSwitchEvent, "expected at least one tariff_switch event in trace")
	require.Equal(t, TraceReasonTariffSwitch, tariffSwitchEvent.Reason)
	require.True(t, strings.Contains(tariffSwitchEvent.Detail, "tariff element became active"),
		"expected detail to contain 'tariff element became active', got: %s", tariffSwitchEvent.Detail)
}

func TestEdge_MeterRollback(t *testing.T) {
	startUTC := time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC)
	endUTC := time.Date(2026, 2, 22, 10, 30, 0, 0, time.UTC)

	tariff := tariffs.Tariff{
		Elements: []tariffs.TariffElement{
			{
				ID: "base",
				PriceComponents: []tariffs.PriceComponent{
					{Type: tariffs.TariffDimensionTypeEnergy},
					{Type: tariffs.TariffDimensionTypeTime},
					{Type: tariffs.TariffDimensionTypeParkingTime},
				},
			},
		},
	}

	meter := []breakpoints.MeterSample{
		{At: startUTC, TotalKWh: 100},
		{At: startUTC.Add(10 * time.Minute), TotalKWh: 101},
		{At: startUTC.Add(20 * time.Minute), TotalKWh: 99},
		{At: endUTC, TotalKWh: 102},
	}

	trace := &Trace{}
	got, err := AccumulateWithTrace(startUTC, endUTC, tariff, meter, nil, nil, nil, nil, trace)
	require.NoError(t, err)
	require.NotEmpty(t, got)

	plain, err := Accumulate(startUTC, endUTC, tariff, meter, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, plain)

	for _, period := range got {
		for _, d := range period.Dimensions {
			if d.Type == DimensionTypeEnergy {
				require.GreaterOrEqual(t, d.Volume, 0.0)
			}
		}
	}
	for _, period := range plain {
		for _, d := range period.Dimensions {
			if d.Type == DimensionTypeEnergy {
				require.GreaterOrEqual(t, d.Volume, 0.0)
			}
		}
	}

	var rollback *TraceEvent
	for i := range trace.Events {
		if trace.Events[i].Reason == TraceReasonMeterRollback {
			e := trace.Events[i]
			rollback = &e
			break
		}
	}
	require.NotNil(t, rollback, "expected meter_rollback event")
	require.Contains(t, rollback.Detail, "meter dropped")
}
