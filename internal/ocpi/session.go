package ocpi

import (
	"context"
	"time"

	"github.com/jaouadou/ocpi-tariff-module/internal/periods"
)

type Session struct {
	ID              string
	ChargingPeriods []periods.ChargingPeriod
	LastUpdated     time.Time
}

type SessionReceiver interface {
	PutSession(ctx context.Context, s Session) error
}
