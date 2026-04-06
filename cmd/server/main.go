package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PatrickSteil/mobidata-poller/internal/db"
	"github.com/PatrickSteil/mobidata-poller/internal/model"
)

func main() {
	dbPath := flag.String("db", "vehicles.db", "Path to SQLite database file")
	addr := flag.String("addr", ":8080", "Listen address")
	flag.Parse()

	store, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/vehicles", makeVehiclesHandler(store))
	mux.HandleFunc("/stats", makeStatsHandler(store))
	mux.HandleFunc("/health", healthHandler)

	log.Printf("server listening on %s  (db: %s)", *addr, *dbPath)
	if err := http.ListenAndServe(*addr, loggingMiddleware(mux)); err != nil {
		log.Fatal(err)
	}
}

// ── /vehicles ─────────────────────────────────────────────────────────────────
//
// Query parameters:
//   lat      float  required  centre latitude
//   lon      float  required  centre longitude
//   radius   float  optional  search radius in metres (default 1000)
//   type     string optional  comma-separated list: bike,escooter,car,cargo_bike,moped,other
//
// Example:
//   GET /vehicles?lat=49.006&lon=8.403&radius=500&type=bike,escooter

func makeVehiclesHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		lat, err := requireFloat(r, "lat")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		lon, err := requireFloat(r, "lon")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		radius := 1000.0
		if s := r.URL.Query().Get("radius"); s != "" {
			if radius, err = strconv.ParseFloat(s, 64); err != nil || radius <= 0 {
				http.Error(w, "radius must be a positive number (metres)", http.StatusBadRequest)
				return
			}
		}

		var types []model.VehicleType
		if s := r.URL.Query().Get("type"); s != "" {
			for _, raw := range strings.Split(s, ",") {
				t := model.VehicleType(strings.TrimSpace(raw))
				if !validType(t) {
					http.Error(w, fmt.Sprintf("unknown type %q — valid: bike,escooter,car,cargo_bike,moped,other", t),
						http.StatusBadRequest)
					return
				}
				types = append(types, t)
			}
		}

		vehicles, err := store.Query(db.QueryParams{
			Lat:    lat,
			Lon:    lon,
			Radius: radius,
			Types:  types,
		})
		if err != nil {
			log.Printf("query error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Return empty array, never null
		if vehicles == nil {
			vehicles = []model.Vehicle{}
		}

		writeJSON(w, map[string]any{
			"count":    len(vehicles),
			"lat":      lat,
			"lon":      lon,
			"radius_m": radius,
			"types":    typeStrings(types),
			"vehicles": vehicles,
		})
	}
}

// ── /stats ────────────────────────────────────────────────────────────────────
//
// Returns vehicle counts grouped by type.

func makeStatsHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := store.Stats()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, stats)
	}
}

// ── /health ───────────────────────────────────────────────────────────────────

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func requireFloat(r *http.Request, key string) (float64, error) {
	s := r.URL.Query().Get(key)
	if s == "" {
		return 0, fmt.Errorf("missing required parameter: %s", key)
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %v", key, err)
	}
	return v, nil
}

func validType(t model.VehicleType) bool {
	switch t {
	case model.TypeBike, model.TypeEScooter, model.TypeCar,
		model.TypeCargo, model.TypeMoped, model.TypeOther:
		return true
	}
	return false
}

func typeStrings(types []model.VehicleType) []string {
	if len(types) == 0 {
		return []string{"all"}
	}
	s := make([]string, len(types))
	for i, t := range types {
		s[i] = string(t)
	}
	return s
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("json encode: %v", err)
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.RequestURI(), time.Since(start).Round(time.Microsecond))
	})
}
