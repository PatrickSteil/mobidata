package main

import (
	"flag"
	"log"
	"sync"
	"time"

	"github.com/PatrickSteil/mobidata-poller/internal/db"
	"github.com/PatrickSteil/mobidata-poller/internal/gbfs"
)

// system is one GBFS operator endpoint.
type system struct {
	ID  string
	URL string
}

// German systems from mobidata-bw.de
var germanSystems = []system{
	// ── Cargo / special bikes ─────────────────────────────────────────────
	{ID: "mikar", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/mikar/gbfs"},
	// {ID: "lastenvelo_fr", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/lastenvelo_fr/gbfs"},

	// ── Carsharing ────────────────────────────────────────────────────────
	{ID: "stadtmobil_stuttgart", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/stadtmobil_stuttgart/gbfs"},
	{ID: "stadtmobil_karlsruhe", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/stadtmobil_karlsruhe/gbfs"},
	{ID: "stadtmobil_rhein-neckar", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/stadtmobil_rhein-neckar/gbfs"},
	{ID: "naturenergie_sharing", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/naturenergie_sharing/gbfs"},
	{ID: "teilauto_neckar-alb", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/teilauto_neckar-alb/gbfs"},
	{ID: "teilauto_biberach", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/teilauto_biberach/gbfs"},
	{ID: "teilauto_schwaebisch_hall", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/teilauto_schwaebisch_hall/gbfs"},
	{ID: "oekostadt_renningen", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/oekostadt_renningen/gbfs"},
	{ID: "gruene-flotte_freiburg", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/gruene-flotte_freiburg/gbfs"},
	{ID: "swu2go", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/swu2go/gbfs"},
	{ID: "conficars_ulm", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/conficars_ulm/gbfs"},
	{ID: "stadtwerk_tauberfranken", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/stadtwerk_tauberfranken/gbfs"},
	{ID: "flinkster_carsharing", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/flinkster_carsharing/gbfs"},
	{ID: "oberschwabenmobil", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/oberschwabenmobil/gbfs"},
	{ID: "ford_carsharing_autohausbaur", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/ford_carsharing_autohausbaur/gbfs"},
	{ID: "ford_carsharing_autohauskauderer", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/ford_carsharing_autohauskauderer/gbfs"},
	{ID: "einfach_unterwegs", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/einfach_unterwegs/gbfs"},
	{ID: "seefahrer_ecarsharing", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/seefahrer_ecarsharing/gbfs"},
	{ID: "stadtwerke_wertheim", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/stadtwerke_wertheim/gbfs"},
	{ID: "hertlein_carsharing", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/hertlein_carsharing/gbfs"},
	{ID: "lara_to_go", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/lara_to_go/gbfs"},
	// {ID: "car_ship", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/car_ship/gbfs"},
	{ID: "stella", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/stella/gbfs"},

	// ── Bikes ────────────────────────────────────────────────────────────
	{ID: "freibe", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/freibe/gbfs"},
	{ID: "lube", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/lube/gbfs"},
	{ID: "regiorad_stuttgart", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/regiorad_stuttgart/gbfs"},
	{ID: "callabike", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/callabike/gbfs"},
	// {ID: "herrenberg_alf", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/herrenberg_alf/gbfs"},
	// {ID: "herrenberg_fare", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/herrenberg_fare/gbfs"},
	// {ID: "herrenberg_guelf", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/herrenberg_guelf/gbfs"},
	// {ID: "herrenberg_stadtrad", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/herrenberg_stadtrad/gbfs"},
	{ID: "hopp_konstanz", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/hopp_konstanz/gbfs"},
	{ID: "nextbike_df", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/nextbike_df/gbfs"},
	{ID: "nextbike_fg", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/nextbike_fg/gbfs"},
	{ID: "nextbike_ds", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/nextbike_ds/gbfs"},
	{ID: "nextbike_vn", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/nextbike_vn/gbfs"},
	{ID: "nextbike_eh", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/nextbike_eh/gbfs"},
	{ID: "nextbike_kk", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/nextbike_kk/gbfs"},
	{ID: "nextbike_nn", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/nextbike_nn/gbfs"},
	{ID: "donkey_bamberg", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/donkey_bamberg/gbfs"},
	{ID: "donkey_kiel", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/donkey_kiel/gbfs"},

	// ── E-Scooters ────────────────────────────────────────────────────────
	{ID: "dott_boblingen", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_boblingen/gbfs"},
	{ID: "dott_friedrichshafen", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_friedrichshafen/gbfs"},
	{ID: "dott_heidelberg", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_heidelberg/gbfs"},
	{ID: "dott_heilbronn", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_heilbronn/gbfs"},
	{ID: "dott_karlsruhe", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_karlsruhe/gbfs"},
	{ID: "dott_lindau", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_lindau/gbfs"},
	{ID: "dott_ludwigsburg", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_ludwigsburg/gbfs"},
	{ID: "dott_mannheim", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_mannheim/gbfs"},
	{ID: "dott_reutlingen", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_reutlingen/gbfs"},
	{ID: "dott_stuttgart", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_stuttgart/gbfs"},
	{ID: "dott_tubingen", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_tubingen/gbfs"},
	{ID: "dott_uberlingen", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_uberlingen/gbfs"},
	{ID: "dott_ulm", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_ulm/gbfs"},
	{ID: "dott_kaiserslautern", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_kaiserslautern/gbfs"},
	{ID: "dott_saarbrucken", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_saarbrucken/gbfs"},
	{ID: "dott_frankfurt", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_frankfurt/gbfs"},
	{ID: "dott_darmstadt", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/dott_darmstadt/gbfs"},
	{ID: "lime_bw", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/lime_bw/gbfs"},
	{ID: "voi_de", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/voi_de/gbfs"},
	{ID: "bolt_karlsruhe", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/bolt_karlsruhe/gbfs"},
	{ID: "bolt_stuttgart", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/bolt_stuttgart/gbfs"},
	{ID: "bolt_reutlingen_tuebingen", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/bolt_reutlingen_tuebingen/gbfs"},
	{ID: "zeus_freiburg", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_freiburg/gbfs"},
	{ID: "zeus_heidelberg", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_heidelberg/gbfs"},
	{ID: "zeus_heilbronn", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_heilbronn/gbfs"},
	{ID: "zeus_kempten", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_kempten/gbfs"},
	{ID: "zeus_konstanz", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_konstanz/gbfs"},
	{ID: "zeus_limburgerhof", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_limburgerhof/gbfs"},
	{ID: "zeus_ludwigsburg", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_ludwigsburg/gbfs"},
	{ID: "zeus_neustadt", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_neustadt/gbfs"},
	{ID: "zeus_pforzheim", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_pforzheim/gbfs"},
	{ID: "zeus_schwabisch_gmund", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_schwabisch_gmund/gbfs"},
	{ID: "zeus_tubingen", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_tubingen/gbfs"},
	{ID: "zeus_tuttlingen", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_tuttlingen/gbfs"},
	{ID: "zeus_villingen_schwenningen", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_villingen_schwenningen/gbfs"},
	{ID: "zeus_renningen_malmsheim", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_renningen_malmsheim/gbfs"},
	{ID: "zeus_goppingen", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_goppingen/gbfs"},
	{ID: "zeus_wurzburg", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeus_wurzburg/gbfs"},
	{ID: "yoio_freiburg", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/yoio_freiburg/gbfs"},
	// {ID: "zeo_bruchsal", URL: "https://api.mobidata-bw.de/sharing/gbfs/v2/zeo_bruchsal/gbfs"},
}

func main() {
	dbPath := flag.String("db", "vehicles.db", "Path to SQLite database file")
	interval := flag.Duration("interval", 60*time.Second, "Poll interval")
	doNotReset := flag.Bool("do-not-reset", false, "Do not drop and recreate tables before every poll (do not use hot-swap mode)")
	flag.Parse()

	store, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	log.Printf("poller started — %d systems, interval %s, db %s",
		len(germanSystems), *interval, *dbPath)

	// Run immediately then on ticker
	pollAll(store, !*doNotReset)
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for range ticker.C {
		pollAll(store, !*doNotReset)
	}
}

// pollAll fetches all systems concurrently and upserts results.
// If reset is true the tables are wiped first so deleted vehicles don't linger.
func pollAll(store *db.Store, reset bool) {
	if reset {
		if err := store.Reset(); err != nil {
			log.Printf("reset failed: %v", err)
		}
	}
	start := time.Now()
	type result struct {
		id    string
		count int
		err   error
	}

	results := make(chan result, len(germanSystems))
	sem := make(chan struct{}, 20) // max 20 concurrent HTTP requests

	var wg sync.WaitGroup
	for _, sys := range germanSystems {
		wg.Add(1)
		go func(s system) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			vehicles, err := gbfs.FetchSystem(s.ID, s.URL)
			if err != nil {
				results <- result{id: s.ID, err: err}
				return
			}
			if err := store.UpsertVehicles(vehicles); err != nil {
				results <- result{id: s.ID, err: err}
				return
			}
			results <- result{id: s.ID, count: len(vehicles)}
		}(sys)
	}

	wg.Wait()
	close(results)

	var total, errors int
	for r := range results {
		if r.err != nil {
			log.Printf("  ✗ %s: %v", r.id, r.err)
			errors++
		} else {
			total += r.count
		}
	}
	log.Printf("poll done in %s — %d vehicles upserted, %d/%d systems failed",
		time.Since(start).Round(time.Millisecond), total, errors, len(germanSystems))
}
