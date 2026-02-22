package tariffs

import "time"

type TariffDimensionType string

const (
	TariffDimensionTypeEnergy      TariffDimensionType = "ENERGY"
	TariffDimensionTypeFlat        TariffDimensionType = "FLAT"
	TariffDimensionTypeParkingTime TariffDimensionType = "PARKING_TIME"
	TariffDimensionTypeTime        TariffDimensionType = "TIME"
	TariffDimensionTypePower       TariffDimensionType = "POWER"
)

type ReservationRestrictionType string

const (
	ReservationRestrictionTypeReservation        ReservationRestrictionType = "RESERVATION"
	ReservationRestrictionTypeReservationExpires ReservationRestrictionType = "RESERVATION_EXPIRES"
)

type Snapshot struct {
	At           time.Time
	Location     *time.Location
	EnergyKWh    float64
	Duration     time.Duration
	CurrentA     float64
	CurrentKnown bool
	PowerKW      float64
	PowerKnown   bool
	Reservation  *ReservationRestrictionType
}

type Tariff struct {
	Elements []TariffElement
}

type TariffElement struct {
	ID              string
	PriceComponents []PriceComponent
	Restrictions    TariffRestrictions
}

type PriceComponent struct {
	Type TariffDimensionType
}

type TariffRestrictions struct {
	StartDate   *string
	EndDate     *string
	StartTime   *string
	EndTime     *string
	MinKWh      *float64
	MaxKWh      *float64
	MinDuration *time.Duration
	MaxDuration *time.Duration
	MinCurrentA *float64
	MaxCurrentA *float64
	MinPowerKW  *float64
	MaxPowerKW  *float64
	DayOfWeek   []time.Weekday
	Reservation *ReservationRestrictionType
}
