package periods

import "time"

type DimensionType string

const (
	DimensionTypeEnergy      DimensionType = "ENERGY"
	DimensionTypeTime        DimensionType = "TIME"
	DimensionTypeParkingTime DimensionType = "PARKING_TIME"
)

type Dimension struct {
	Type   DimensionType
	Volume float64
}

type ChargingPeriod struct {
	Start      time.Time
	Dimensions []Dimension
}
