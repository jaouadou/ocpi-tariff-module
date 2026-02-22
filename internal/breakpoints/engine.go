package breakpoints

import (
	"math"
	"sort"
	"time"
)

type MeterSample struct {
	At       time.Time
	TotalKWh float64
}

type EnergyThreshold struct {
	Kind string
	KWh  float64
}

func Breakpoints(startUTC, endUTC time.Time, meter []MeterSample, calendar []time.Time, thresholds []EnergyThreshold) []time.Time {
	if !startUTC.Before(endUTC) {
		return nil
	}

	startUTC = startUTC.UTC()
	endUTC = endUTC.UTC()

	candidates := make(map[int64]time.Time)
	addCandidate(candidates, startUTC)
	addCandidate(candidates, endUTC)

	sortedMeter := sortMeterSamples(meter)
	for _, sample := range sortedMeter {
		addCandidate(candidates, sample.At)
	}

	for _, instant := range calendar {
		addCandidate(candidates, instant)
	}

	for _, instant := range energyThresholdCrossings(sortedMeter, thresholds) {
		addCandidate(candidates, instant)
	}

	out := make([]time.Time, 0, len(candidates))
	for _, instant := range candidates {
		if instant.After(startUTC) && (instant.Before(endUTC) || instant.Equal(endUTC)) {
			out = append(out, instant.UTC())
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Before(out[j])
	})

	return out
}

func sortMeterSamples(meter []MeterSample) []MeterSample {
	if len(meter) == 0 {
		return nil
	}

	sorted := make([]MeterSample, len(meter))
	copy(sorted, meter)

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

func energyThresholdCrossings(samples []MeterSample, thresholds []EnergyThreshold) []time.Time {
	if len(samples) < 2 || len(thresholds) == 0 {
		return nil
	}

	out := make([]time.Time, 0)

	for i := 1; i < len(samples); i++ {
		a := samples[i-1]
		b := samples[i]

		if b.TotalKWh == a.TotalKWh {
			continue
		}

		deltaKWh := b.TotalKWh - a.TotalKWh
		if deltaKWh <= 0 {
			continue
		}

		deltaDuration := b.At.Sub(a.At)

		for _, threshold := range thresholds {
			if !isEnergyThresholdKind(threshold.Kind) {
				continue
			}
			if !(a.TotalKWh < threshold.KWh && threshold.KWh <= b.TotalKWh) {
				continue
			}

			ratio := (threshold.KWh - a.TotalKWh) / deltaKWh
			offsetNanos := int64(math.Round(ratio * float64(deltaDuration)))
			crossing := a.At.Add(time.Duration(offsetNanos)).UTC()
			out = append(out, crossing)
		}
	}

	return out
}

func isEnergyThresholdKind(kind string) bool {
	return kind == "min" || kind == "max"
}

func addCandidate(candidates map[int64]time.Time, instant time.Time) {
	instant = instant.UTC()
	candidates[instant.UnixNano()] = instant
}
