package gbfs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PatrickSteil/mobidata-poller/internal/model"
)

// ── GBFS discovery structs ────────────────────────────────────────────────────

type gbfsRoot struct {
	Data map[string]struct {
		Feeds []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"feeds"`
	} `json:"data"`
}

// ── free_bike_status structs ──────────────────────────────────────────────────

type freeBikeStatus struct {
	LastUpdated int64 `json:"last_updated"`
	Data        struct {
		Bikes []rawBike `json:"bikes"`
	} `json:"data"`
}

type rawBike struct {
	BikeID        string  `json:"bike_id"`
	Lat           float64 `json:"lat"`
	Lon           float64 `json:"lon"`
	IsReserved    bool    `json:"is_reserved"`
	IsDisabled    bool    `json:"is_disabled"`
	VehicleTypeID string  `json:"vehicle_type_id"`
}

// ── vehicle_types structs ─────────────────────────────────────────────────────

type vehicleTypeFeed struct {
	Data struct {
		VehicleTypes []rawVehicleType `json:"vehicle_types"`
	} `json:"data"`
}

type rawVehicleType struct {
	VehicleTypeID  string `json:"vehicle_type_id"`
	FormFactor     string `json:"form_factor"`     // "bicycle", "scooter", "car", "moped", "cargo_bicycle"
	PropulsionType string `json:"propulsion_type"` // "electric", "human", ...
}

// ── HTTP client ───────────────────────────────────────────────────────────────

var httpClient = &http.Client{Timeout: 15 * time.Second}

func get(url string, dst any) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

// ── Public API ────────────────────────────────────────────────────────────────

// FetchSystem discovers and fetches all vehicles for one GBFS system.
// gbfsURL is the root /gbfs endpoint.
func FetchSystem(systemID, gbfsURL string) ([]model.Vehicle, error) {
	var root gbfsRoot
	if err := get(gbfsURL, &root); err != nil {
		return nil, fmt.Errorf("discover %s: %w", systemID, err)
	}

	// The root may be keyed by language ("en", "de", ...) or directly contain feeds.
	// We flatten all language variants and look for the feeds we need.
	feedURLs := map[string]string{}
	for _, lang := range root.Data {
		for _, f := range lang.Feeds {
			feedURLs[f.Name] = f.URL
		}
	}

	statusURL, ok := feedURLs["free_bike_status"]
	if !ok {
		return nil, fmt.Errorf("%s: no free_bike_status feed", systemID)
	}

	// Optional: vehicle_types feed for better classification
	typeMap := map[string]model.VehicleType{}
	if vtURL, ok := feedURLs["vehicle_types"]; ok {
		var vtFeed vehicleTypeFeed
		if err := get(vtURL, &vtFeed); err == nil {
			for _, vt := range vtFeed.Data.VehicleTypes {
				typeMap[vt.VehicleTypeID] = classifyFormFactor(vt.FormFactor, vt.PropulsionType)
			}
		}
	}

	var status freeBikeStatus
	if err := get(statusURL, &status); err != nil {
		return nil, fmt.Errorf("%s: free_bike_status: %w", systemID, err)
	}

	now := time.Now().Unix()
	vehicles := make([]model.Vehicle, 0, len(status.Data.Bikes))
	for _, b := range status.Data.Bikes {
		vtype, ok := typeMap[b.VehicleTypeID]
		if !ok {
			vtype = classifyByName(systemID, b.VehicleTypeID)
		}
		vehicles = append(vehicles, model.Vehicle{
			ID:          b.BikeID,
			Operator:    systemID,
			Lat:         b.Lat,
			Lon:         b.Lon,
			Type:        vtype,
			IsReserved:  b.IsReserved,
			IsDisabled:  b.IsDisabled,
			LastUpdated: status.LastUpdated,
			SeenAt:      now,
		})
	}
	return vehicles, nil
}

// classifyFormFactor maps GBFS form_factor + propulsion to our VehicleType.
func classifyFormFactor(formFactor, propulsion string) model.VehicleType {
	switch strings.ToLower(formFactor) {
	case "bicycle":
		return model.TypeBike
	case "cargo_bicycle":
		return model.TypeCargo
	case "scooter", "scooter_standing":
		return model.TypeEScooter
	case "moped":
		return model.TypeMoped
	case "car":
		return model.TypeCar
	}
	// fallback: electric scooters often named just "scooter"
	if strings.Contains(strings.ToLower(propulsion), "electric") {
		return model.TypeEScooter
	}
	return model.TypeOther
}

// classifyByName uses system-id / vehicle_type_id heuristics when no vehicle_types feed exists.
func classifyByName(systemID, vehicleTypeID string) model.VehicleType {
	id := strings.ToLower(systemID + " " + vehicleTypeID)
	switch {
	case contains(id, "scooter", "escooter", "e-scooter"):
		return model.TypeEScooter
	case contains(id, "car", "auto", "pkw", "carsharing"):
		return model.TypeCar
	case contains(id, "cargo", "lastenrad", "lastenfahrrad", "cargobike"):
		return model.TypeCargo
	case contains(id, "moped"):
		return model.TypeMoped
	case contains(id, "bike", "bicycle", "rad", "cycle", "velo"):
		return model.TypeBike
	}
	// Operator-level fallback
	switch {
	case hasPrefix(systemID, "dott", "voi", "lime", "bolt", "zeus", "bird", "yoio", "zeo", "hopp"):
		return model.TypeEScooter
	case hasPrefix(systemID, "nextbike", "regiorad", "callabike", "donkey", "freibe", "lube", "herrenberg"):
		return model.TypeBike
	case hasPrefix(systemID, "stadtmobil", "teilauto", "flinkster", "car_ship", "ford_carsharing",
		"conficars", "oberschwabenmobil", "naturenergie", "seefahrer", "hertlein", "lara", "stella",
		"stadtwerk", "stadtwerke", "swu2go", "einfach", "oekostadt", "gruene"):
		return model.TypeCar
	case hasPrefix(systemID, "lastenvelo", "mikar", "carvelo"):
		return model.TypeCargo
	}
	return model.TypeOther
}

func contains(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func hasPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
