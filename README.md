# mobidata-poller

A Go project that polls live vehicle-sharing data (bikes, e-scooters, cars, cargo bikes) from
[mobidata-bw.de](https://api.mobidata-bw.de/) and stores it in a local SQLite database.

Two binaries:

| Binary | Purpose |
|---|---|
| `cmd/poller` | Polls ~80 German GBFS feeds every 60 s, upserts to SQLite |
| `cmd/server` | HTTP server to query vehicles by location + type |

## Build

```bash
make
```

## Run

### 1. Start the poller

```bash
./bin/poller -db vehicles.db -interval 60s
```

Flags:

| Flag | Default | Description |
|---|---|---|
| `-db` | `vehicles.db` | SQLite file path |
| `-interval` | `60s` | Poll interval |

The poller runs all ~80 system fetches concurrently (max 20 in-flight HTTP requests).
On first run it immediately polls; subsequent polls happen on the interval tick.

### 2. Start the query server

```bash
./bin/server -db vehicles.db -addr :8080
```

Flags:

| Flag | Default | Description |
|---|---|---|
| `-db` | `vehicles.db` | SQLite file path (same file as poller) |
| `-addr` | `:8080` | Listen address |

Both binaries can run simultaneously — the poller writes via WAL mode, the server reads concurrently without blocking.

## HTTP API

### `GET /vehicles`

Returns vehicles within a radius of a given coordinate.

**Query parameters:**

| Parameter | Required | Default | Description |
|---|---|---|---|
| `lat` | ✓ | — | Centre latitude |
| `lon` | ✓ | — | Centre longitude |
| `radius` | — | `1000` | Search radius in **metres** |
| `type` | — | all | Comma-separated type filter |

**Valid type values:** `bike`, `escooter`, `car`, `cargo_bike`, `moped`, `other`

**Examples:**

```bash
# All vehicles within 500 m of Karlsruhe market square
curl "http://localhost:8080/vehicles?lat=49.009&lon=8.404&radius=500"

# Only bikes and e-scooters within 1 km of Stuttgart centre
curl "http://localhost:8080/vehicles?lat=48.775&lon=9.182&radius=1000&type=bike,escooter"

# Only carsharing within 2 km
curl "http://localhost:8080/vehicles?lat=48.775&lon=9.182&radius=2000&type=car"
```

**Response:**

```json
{
  "count": 3,
  "lat": 49.009,
  "lon": 8.404,
  "radius_m": 500,
  "types": ["bike", "escooter"],
  "vehicles": [
    {
      "id": "DOC:Vehicle:abc123",
      "operator": "dott_karlsruhe",
      "lat": 49.0085,
      "lon": 8.4012,
      "type": "escooter",
      "is_reserved": false,
      "is_disabled": false,
      "last_updated": 1775462172,
      "seen_at": 1775462200
    }
  ]
}
```

### `GET /stats`

Returns the total count of vehicles in the database grouped by type.

```bash
curl http://localhost:8080/stats
```

```json
{
  "bike": 1402,
  "car": 318,
  "cargo_bike": 24,
  "escooter": 4871,
  "moped": 0,
  "other": 12
}
```

### `GET /health`

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

## Vehicle Types

| Type | Examples |
|---|---|
| `bike` | nextbike, regiorad, callabike, donkey, herrenberg |
| `escooter` | dott, lime, voi, bolt, zeus, bird, yoio, zeo |
| `car` | stadtmobil, teilauto, flinkster, ford_carsharing, swu2go |
| `cargo_bike` | lastenvelo_fr, mikar, carvelo2go |
| `moped` | (detected from GBFS vehicle_types feed) |
| `other` | Anything not classifiable |

Type detection uses the GBFS `vehicle_types` feed when available, with a fallback heuristic based
on the system ID and `vehicle_type_id` field.

## Project Structure

```
.
├── cmd/
│   ├── poller/main.go      ← polling binary
│   └── server/main.go      ← HTTP server binary
├── internal/
│   ├── db/store.go         ← SQLite store + geo query
│   ├── gbfs/client.go      ← GBFS HTTP client + type classifier
│   └── model/vehicle.go    ← shared Vehicle struct
├── go.mod
└── README.md
```
