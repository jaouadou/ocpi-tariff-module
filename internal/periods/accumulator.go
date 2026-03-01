package periods

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jaouadou/ocpi-tariff-module/internal/breakpoints"
	"github.com/jaouadou/ocpi-tariff-module/internal/tariffs"
)

func Accumulate(
	startUTC, endUTC time.Time,
	tariff tariffs.Tariff,
	meter []breakpoints.MeterSample,
	power []breakpoints.PowerSample,
	currentSamples []breakpoints.CurrentSample,
	calendar []time.Time,
	thresholds []breakpoints.EnergyThreshold,
) ([]ChargingPeriod, error) {
	if !startUTC.Before(endUTC) {
		return nil, errors.New("invalid session window")
	}
	if len(meter) == 0 {
		return nil, errors.New("at least one meter sample is required")
	}

	startUTC = startUTC.UTC()
	endUTC = endUTC.UTC()

	normalized := normalizeMeterSamples(meter)
	normalizedPower := normalizePowerSamples(power)
	normalizedCurrent := normalizeCurrentSamples(currentSamples)
	startTotalKWh, err := totalKWhAt(normalized, startUTC)
	if err != nil {
		return nil, err
	}

	bps := breakpoints.Breakpoints(startUTC, endUTC, normalized, calendar, thresholds)
	boundariesByNanos := make(map[int64]time.Time, len(bps)+len(normalizedPower)+len(normalizedCurrent)+2)
	addBoundary(boundariesByNanos, startUTC)
	for _, bp := range bps {
		addBoundary(boundariesByNanos, bp)
	}
	for _, sample := range normalizedPower {
		addBoundary(boundariesByNanos, sample.At)
	}
	for _, sample := range normalizedCurrent {
		addBoundary(boundariesByNanos, sample.At)
	}
	addBoundary(boundariesByNanos, endUTC)

	boundaries := make([]time.Time, 0, len(boundariesByNanos))
	for _, boundary := range boundariesByNanos {
		if boundary.Before(startUTC) || boundary.After(endUTC) {
			continue
		}
		boundaries = append(boundaries, boundary)
	}
	sort.Slice(boundaries, func(i, j int) bool { return boundaries[i].Before(boundaries[j]) })

	periods := make([]ChargingPeriod, 0)
	var current *ChargingPeriod
	currentKey := ""

	for i := 1; i < len(boundaries); i++ {
		a := boundaries[i-1]
		b := boundaries[i]

		energyStart, err := totalKWhAt(normalized, a)
		if err != nil {
			return nil, err
		}
		energyEnd, err := totalKWhAt(normalized, b)
		if err != nil {
			return nil, err
		}

		deltaKWh := energyEnd - energyStart
		if deltaKWh < 0 {
			deltaKWh = 0
		}

		sessionEnergyAtA := energyStart - startTotalKWh
		if sessionEnergyAtA < 0 {
			sessionEnergyAtA = 0
		}

		snap := tariffs.Snapshot{
			At:           a,
			EnergyKWh:    sessionEnergyAtA,
			Duration:     a.Sub(startUTC),
			PowerKW:      powerAt(normalizedPower, a),
			PowerKnown:   powerKnownAt(normalizedPower, a),
			CurrentA:     currentAt(normalizedCurrent, a),
			CurrentKnown: currentKnownAt(normalizedCurrent, a),
		}
		selected := tariffs.SelectActiveElements(tariff, snap)

		charging := deltaKWh > 0
		key := selectionKey(selected, charging)

		if current == nil || key != currentKey {
			periods = append(periods, ChargingPeriod{Start: a.UTC()})
			current = &periods[len(periods)-1]
			currentKey = key
		}

		if charging {
			addDimension(current, DimensionTypeEnergy, deltaKWh)
			addDimension(current, DimensionTypeTime, b.Sub(a).Hours())
		} else {
			addDimension(current, DimensionTypeParkingTime, b.Sub(a).Hours())
		}
	}

	for i := range periods {
		periods[i].Dimensions = normalizeDimensions(periods[i].Dimensions)
	}

	return periods, nil
}

func addBoundary(byNanos map[int64]time.Time, t time.Time) {
	t = t.UTC()
	byNanos[t.UnixNano()] = t
}

func selectionKey(selected map[tariffs.TariffDimensionType]tariffs.TariffElement, charging bool) string {
	state := "parking"
	if charging {
		state = "charging"
	}

	energyID := selected[tariffs.TariffDimensionTypeEnergy].ID
	timeID := selected[tariffs.TariffDimensionTypeTime].ID
	parkingID := selected[tariffs.TariffDimensionTypeParkingTime].ID

	return fmt.Sprintf("%s|E:%s|T:%s|P:%s", state, energyID, timeID, parkingID)
}

func addDimension(period *ChargingPeriod, typ DimensionType, volume float64) {
	if volume <= 0 {
		return
	}

	for i := range period.Dimensions {
		if period.Dimensions[i].Type == typ {
			period.Dimensions[i].Volume += volume
			return
		}
	}

	period.Dimensions = append(period.Dimensions, Dimension{Type: typ, Volume: volume})
}

func normalizeDimensions(in []Dimension) []Dimension {
	if len(in) == 0 {
		return nil
	}

	order := map[DimensionType]int{
		DimensionTypeEnergy:      0,
		DimensionTypeTime:        1,
		DimensionTypeParkingTime: 2,
	}

	out := make([]Dimension, len(in))
	copy(out, in)
	sort.Slice(out, func(i, j int) bool {
		return order[out[i].Type] < order[out[j].Type]
	})

	return out
}

func normalizeMeterSamples(in []breakpoints.MeterSample) []breakpoints.MeterSample {
	if len(in) == 0 {
		return nil
	}

	sorted := make([]breakpoints.MeterSample, len(in))
	copy(sorted, in)

	for i := range sorted {
		sorted[i].At = sorted[i].At.UTC()
	}

	sort.Slice(sorted, func(i, j int) bool {
		if !sorted[i].At.Equal(sorted[j].At) {
			return sorted[i].At.Before(sorted[j].At)
		}
		return sorted[i].TotalKWh < sorted[j].TotalKWh
	})

	out := make([]breakpoints.MeterSample, 0, len(sorted))
	for _, s := range sorted {
		if len(out) > 0 && s.At.Equal(out[len(out)-1].At) {
			out[len(out)-1] = s
			continue
		}
		out = append(out, s)
	}

	return out
}

func normalizePowerSamples(in []breakpoints.PowerSample) []breakpoints.PowerSample {
	if len(in) == 0 {
		return nil
	}

	sorted := make([]breakpoints.PowerSample, len(in))
	copy(sorted, in)

	for i := range sorted {
		sorted[i].At = sorted[i].At.UTC()
	}

	sort.Slice(sorted, func(i, j int) bool {
		if !sorted[i].At.Equal(sorted[j].At) {
			return sorted[i].At.Before(sorted[j].At)
		}
		return sorted[i].PowerKW < sorted[j].PowerKW
	})

	out := make([]breakpoints.PowerSample, 0, len(sorted))
	for _, sample := range sorted {
		if len(out) > 0 && sample.At.Equal(out[len(out)-1].At) {
			out[len(out)-1] = sample
			continue
		}
		out = append(out, sample)
	}

	return out
}

func normalizeCurrentSamples(in []breakpoints.CurrentSample) []breakpoints.CurrentSample {
	if len(in) == 0 {
		return nil
	}

	sorted := make([]breakpoints.CurrentSample, len(in))
	copy(sorted, in)

	for i := range sorted {
		sorted[i].At = sorted[i].At.UTC()
	}

	sort.Slice(sorted, func(i, j int) bool {
		if !sorted[i].At.Equal(sorted[j].At) {
			return sorted[i].At.Before(sorted[j].At)
		}
		return sorted[i].CurrentA < sorted[j].CurrentA
	})

	out := make([]breakpoints.CurrentSample, 0, len(sorted))
	for _, sample := range sorted {
		if len(out) > 0 && sample.At.Equal(out[len(out)-1].At) {
			out[len(out)-1] = sample
			continue
		}
		out = append(out, sample)
	}

	return out
}

func powerAt(samples []breakpoints.PowerSample, t time.Time) float64 {
	if len(samples) == 0 {
		return 0
	}

	t = t.UTC()
	idx := sort.Search(len(samples), func(i int) bool {
		return samples[i].At.After(t)
	})
	if idx == 0 {
		return 0
	}
	return samples[idx-1].PowerKW
}

func powerKnownAt(samples []breakpoints.PowerSample, t time.Time) bool {
	if len(samples) == 0 {
		return false
	}

	t = t.UTC()
	idx := sort.Search(len(samples), func(i int) bool {
		return samples[i].At.After(t)
	})
	return idx > 0
}

func currentAt(samples []breakpoints.CurrentSample, t time.Time) float64 {
	if len(samples) == 0 {
		return 0
	}

	t = t.UTC()
	idx := sort.Search(len(samples), func(i int) bool {
		return samples[i].At.After(t)
	})
	if idx == 0 {
		return 0
	}
	return samples[idx-1].CurrentA
}

func currentKnownAt(samples []breakpoints.CurrentSample, t time.Time) bool {
	if len(samples) == 0 {
		return false
	}

	t = t.UTC()
	idx := sort.Search(len(samples), func(i int) bool {
		return samples[i].At.After(t)
	})
	return idx > 0
}

func totalKWhAt(samples []breakpoints.MeterSample, t time.Time) (float64, error) {
	if len(samples) == 0 {
		return 0, errors.New("missing meter samples")
	}

	t = t.UTC()
	if t.Before(samples[0].At) || t.Equal(samples[0].At) {
		return samples[0].TotalKWh, nil
	}
	last := samples[len(samples)-1]
	if t.After(last.At) || t.Equal(last.At) {
		return last.TotalKWh, nil
	}

	idx := sort.Search(len(samples), func(i int) bool {
		return !samples[i].At.Before(t)
	})

	if idx < len(samples) && samples[idx].At.Equal(t) {
		return samples[idx].TotalKWh, nil
	}
	if idx == 0 || idx >= len(samples) {
		return 0, fmt.Errorf("cannot interpolate meter at %s", t.Format(time.RFC3339Nano))
	}

	a := samples[idx-1]
	b := samples[idx]
	deltaT := b.At.Sub(a.At).Seconds()
	if deltaT <= 0 {
		return a.TotalKWh, nil
	}
	ratio := t.Sub(a.At).Seconds() / deltaT
	return a.TotalKWh + ratio*(b.TotalKWh-a.TotalKWh), nil
}
