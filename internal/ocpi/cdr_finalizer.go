package ocpi

import (
	"sync"
	"time"

	"github.com/jaouadou/ocpi-tariff-module/internal/periods"
)

type Finalizer struct {
	mu     sync.Mutex
	sealed map[string]CDR
}

func NewFinalizer() *Finalizer {
	return &Finalizer{sealed: make(map[string]CDR)}
}

func (f *Finalizer) TryFinalize(sessionID string, startUTC, endUTC, watermarkUTC time.Time, p []periods.ChargingPeriod, now time.Time) (CDR, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if cdr, ok := f.sealed[sessionID]; ok {
		return cdr, true
	}

	if watermarkUTC.Before(endUTC) {
		return CDR{}, false
	}

	sealed := CDR{
		SessionID:       sessionID,
		Start:           startUTC,
		End:             endUTC,
		ChargingPeriods: cloneChargingPeriods(p),
		FinalizedAt:     now,
	}

	f.sealed[sessionID] = sealed
	return sealed, true
}

func cloneChargingPeriods(in []periods.ChargingPeriod) []periods.ChargingPeriod {
	if len(in) == 0 {
		return nil
	}

	out := make([]periods.ChargingPeriod, len(in))
	for i := range in {
		out[i] = in[i]
		if len(in[i].Dimensions) == 0 {
			out[i].Dimensions = nil
			continue
		}
		out[i].Dimensions = make([]periods.Dimension, len(in[i].Dimensions))
		copy(out[i].Dimensions, in[i].Dimensions)
	}

	return out
}
