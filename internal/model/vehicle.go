package model

// VehicleType classifies a sharing vehicle.
type VehicleType string

const (
	TypeBike     VehicleType = "bike"
	TypeEScooter VehicleType = "escooter"
	TypeCar      VehicleType = "car"
	TypeCargo    VehicleType = "cargo_bike"
	TypeMoped    VehicleType = "moped"
	TypeOther    VehicleType = "other"
)

// Vehicle is the minimal record we store per vehicle snapshot.
type Vehicle struct {
	ID          string      `json:"id"`       // original bike_id from GBFS
	Operator    string      `json:"operator"` // system id, e.g. "dott_karlsruhe"
	Lat         float64     `json:"lat"`
	Lon         float64     `json:"lon"`
	Type        VehicleType `json:"type"`
	IsReserved  bool        `json:"is_reserved"`
	IsDisabled  bool        `json:"is_disabled"`
	LastUpdated int64       `json:"last_updated"` // unix epoch from GBFS feed
	SeenAt      int64       `json:"seen_at"`      // unix epoch when we polled
}
