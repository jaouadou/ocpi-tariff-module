package ocpi

import (
	"context"
	"reflect"
	"sync"

	"github.com/jaouadou/ocpi-tariff-module/internal/periods"
)

type SessionProjector struct {
	receiver SessionReceiver

	mu                sync.Mutex
	projectionVersion map[string]uint64
	lastProjected     map[string]Session
}

func NewSessionProjector(receiver SessionReceiver) *SessionProjector {
	return &SessionProjector{
		receiver:          receiver,
		projectionVersion: make(map[string]uint64),
		lastProjected:     make(map[string]Session),
	}
}

func (p *SessionProjector) Emit(ctx context.Context, s Session) error {
	if s.ID == "" {
		return nil
	}

	next := cloneSession(s)

	p.mu.Lock()
	last, hasLast := p.lastProjected[next.ID]
	if hasLast && reflect.DeepEqual(last, next) {
		p.mu.Unlock()
		return nil
	}
	p.projectionVersion[next.ID]++
	p.mu.Unlock()

	if err := p.receiver.PutSession(ctx, next); err != nil {
		return err
	}

	p.mu.Lock()
	p.lastProjected[next.ID] = next
	p.mu.Unlock()

	return nil
}

func cloneSession(s Session) Session {
	out := s
	if len(s.ChargingPeriods) == 0 {
		out.ChargingPeriods = nil
		return out
	}

	out.ChargingPeriods = make([]periods.ChargingPeriod, len(s.ChargingPeriods))
	for i := range s.ChargingPeriods {
		out.ChargingPeriods[i] = s.ChargingPeriods[i]
		if len(s.ChargingPeriods[i].Dimensions) == 0 {
			out.ChargingPeriods[i].Dimensions = nil
			continue
		}
		out.ChargingPeriods[i].Dimensions = make([]periods.Dimension, len(s.ChargingPeriods[i].Dimensions))
		copy(out.ChargingPeriods[i].Dimensions, s.ChargingPeriods[i].Dimensions)
	}

	return out
}
