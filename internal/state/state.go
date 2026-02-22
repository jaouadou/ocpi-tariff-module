package state

import (
	"sort"
	"time"

	"github.com/jaouadou/ocpi-tariff-module/internal/breakpoints"
)

type State uint8

const (
	Charging State = iota
	Parking
)

func ClassifyInterval(start, end breakpoints.MeterSample) State {
	if end.TotalKWh-start.TotalKWh > 0 {
		return Charging
	}

	return Parking
}

func ChargingParkingBreakpoints(samples []breakpoints.MeterSample) []time.Time {
	sorted := sortSamples(samples)
	if len(sorted) < 3 {
		return nil
	}

	out := make([]time.Time, 0)
	prevState, ok := intervalState(sorted[0], sorted[1])
	if !ok {
		return nil
	}

	for i := 2; i < len(sorted); i++ {
		currentState, valid := intervalState(sorted[i-1], sorted[i])
		if !valid {
			continue
		}

		if currentState != prevState {
			breakpoint := sorted[i-1].At.UTC()
			if len(out) == 0 || breakpoint.After(out[len(out)-1]) {
				out = append(out, breakpoint)
			}
		}

		prevState = currentState
	}

	return out
}

func intervalState(start, end breakpoints.MeterSample) (State, bool) {
	if !end.At.After(start.At) {
		return Parking, false
	}

	return ClassifyInterval(start, end), true
}

func sortSamples(samples []breakpoints.MeterSample) []breakpoints.MeterSample {
	if len(samples) == 0 {
		return nil
	}

	sorted := make([]breakpoints.MeterSample, len(samples))
	copy(sorted, samples)

	for i := range sorted {
		sorted[i].At = sorted[i].At.UTC()
	}

	sort.Slice(sorted, func(i, j int) bool {
		if !sorted[i].At.Equal(sorted[j].At) {
			return sorted[i].At.Before(sorted[j].At)
		}

		return sorted[i].TotalKWh < sorted[j].TotalKWh
	})

	return sorted
}
