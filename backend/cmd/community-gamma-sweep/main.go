// Command community-gamma-sweep is a READ-ONLY tuning tool for the Leiden
// resolution parameter (gamma) behind the persisted artist community partition
// (PSY-1262 / PSY-1320). It loads the SAME filtered edge set the nightly
// compute uses (catalog.LoadCommunityGraphEdges), runs LeidenCommunities across
// a sweep of gamma values, and reports — per gamma — the global partition shape
// (community count, size distribution, top communities with their "Around
// {artist}" anchors) plus a per-metro projection simulating the scene graph's
// cluster rules (first-class floor 6, palette cap 8, remainder → "other").
// It NEVER writes; use it to choose + justify LeidenResolution before the
// scene-graph default flips to community clustering (PSY-1320's blocker).
//
// The per-metro projection uses ALL artists keyed to the metro
// (artists.metro = CBSA), a superset of the scene graph's top-75 roster —
// close enough to eyeball blob-vs-dust, not a pixel-exact UI simulation.
//
// Meaningful output requires SCENE-SCALE data: run it against stage — pull the
// DATABASE_PUBLIC_URL into a temp env (stage-ops convention) and pass --env.
//
// Usage:
//
//	go run ./cmd/community-gamma-sweep --env /tmp/stage.env
//	go run ./cmd/community-gamma-sweep --env /tmp/stage.env --gammas 0.8,1.0,1.2 --metros 38060,16980
package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	catalogsvc "psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/geo"
)

var (
	envFile   string
	gammaCSV  string
	metroCSV  string
	topN      int
	leidenSee int64
)

// defaultGammas brackets the untuned LeidenResolution = 1.0 placeholder.
var defaultGammas = []float64{0.5, 0.8, 1.0, 1.5, 2.0, 3.0}

// defaultMetros are the eyeball scenes named on PSY-1320: Phoenix (38060),
// Chicago (16980).
const defaultMetros = "38060,16980"

// Scene-graph cluster rules mirrored from scene.go (sceneClusterMinSize /
// sceneClusterMaxFirstClass). Kept as locals: this is a reporting
// approximation, not a contract.
const (
	clusterMinSize       = 6
	clusterMaxFirstClass = 8
)

func main() {
	flag.StringVar(&envFile, "env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.StringVar(&gammaCSV, "gammas", "", "Comma-separated gamma sweep (default 0.5,0.8,1.0,1.5,2.0,3.0)")
	flag.StringVar(&metroCSV, "metros", defaultMetros, "Comma-separated CBSA codes to project per-scene")
	flag.IntVar(&topN, "top", 10, "Top-N communities to list per gamma")
	flag.Int64Var(&leidenSee, "seed", 42, "Leiden RNG seed (default matches the nightly compute)")
	flag.Parse()

	gammas := defaultGammas
	if gammaCSV != "" {
		gammas = nil
		for _, part := range strings.Split(gammaCSV, ",") {
			g, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
			if err != nil || g <= 0 {
				log.Fatalf("invalid gamma %q", part)
			}
			gammas = append(gammas, g)
		}
	}

	loadEnv()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("connect db: %v", err)
	}
	database := db.GetDB()

	fmt.Println("=== Leiden Gamma Sweep (READ-ONLY) — PSY-1262/PSY-1320 ===")
	fmt.Printf("Target: db=%s\n\n", redactDBHost(cfg.Database.URL))

	svc := catalogsvc.NewRadioService(database)
	edges, err := svc.LoadCommunityGraphEdges()
	if err != nil {
		log.Fatalf("load community graph edges: %v", err)
	}
	if len(edges) == 0 {
		log.Fatalf("filtered similarity graph is empty — nothing to sweep (affinity/backbone not computed on this DB)")
	}

	// Node strength in the filtered graph anchors labels, mirroring
	// ComputeArtistCommunities.
	strength := make(map[uint]float64, len(edges)*2)
	nodeSet := make(map[uint]struct{}, len(edges)*2)
	for _, e := range edges {
		strength[e.A] += e.Weight
		strength[e.B] += e.Weight
		nodeSet[e.A] = struct{}{}
		nodeSet[e.B] = struct{}{}
	}
	fmt.Printf("Input graph: %d edges over %d artists (radio restricted to backbone-significant)\n\n", len(edges), len(nodeSet))

	names := loadArtistNames(database, nodeSet)
	metros := loadMetroRosters(database, metroCSV)

	for _, gamma := range gammas {
		assignment := catalogsvc.LeidenCommunities(edges, gamma, leidenSee)
		report(gamma, assignment, strength, names, metros)
	}

	fmt.Println("Done. No writes were performed.")
}

// metroRoster is one eyeball scene: every artist keyed to the CBSA.
type metroRoster struct {
	cbsa      string
	name      string
	artistIDs []uint
}

func report(gamma float64, assignment map[uint]int, strength map[uint]float64, names map[uint]string, metros []metroRoster) {
	// Community sizes + label anchors (highest strength, ties to lower ID —
	// same rule as the nightly compute).
	sizes := map[int]int{}
	anchor := map[int]uint{}
	for artistID, comm := range assignment {
		sizes[comm]++
		best, ok := anchor[comm]
		if !ok || strength[artistID] > strength[best] || (strength[artistID] == strength[best] && artistID < best) {
			anchor[comm] = artistID
		}
	}

	ordered := make([]int, 0, len(sizes))
	for c := range sizes {
		ordered = append(ordered, c)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if sizes[ordered[i]] != sizes[ordered[j]] {
			return sizes[ordered[i]] > sizes[ordered[j]]
		}
		return ordered[i] < ordered[j]
	})

	sizeList := make([]int, 0, len(ordered))
	singletons, small, mid, large := 0, 0, 0, 0
	for _, c := range ordered {
		n := sizes[c]
		sizeList = append(sizeList, n)
		switch {
		case n == 1:
			singletons++
		case n < clusterMinSize:
			small++
		case n <= 50:
			mid++
		default:
			large++
		}
	}

	fmt.Printf("--- gamma = %.2f ---\n", gamma)
	fmt.Printf("communities=%d assigned=%d | max=%d median=%d | singletons=%d small(2-5)=%d mid(6-50)=%d large(>50)=%d\n",
		len(sizes), len(assignment), sizeList[0], median(sizeList), singletons, small, mid, large)

	fmt.Printf("top %d:\n", topN)
	for i, c := range ordered {
		if i >= topN {
			break
		}
		fmt.Printf("  %5d members  Around %s\n", sizes[c], nameOf(names, anchor[c]))
	}

	for _, m := range metros {
		reportMetro(m, assignment, sizes, anchor, names)
	}
	fmt.Println()
}

// reportMetro projects the partition onto one metro roster and simulates the
// scene graph's first-class/other split.
func reportMetro(m metroRoster, assignment map[uint]int, globalSizes map[int]int, anchor map[int]uint, names map[uint]string) {
	inScene := map[int]int{}
	assigned := 0
	for _, id := range m.artistIDs {
		if comm, ok := assignment[id]; ok {
			inScene[comm]++
			assigned++
		}
	}

	ordered := make([]int, 0, len(inScene))
	for c := range inScene {
		ordered = append(ordered, c)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if inScene[ordered[i]] != inScene[ordered[j]] {
			return inScene[ordered[i]] > inScene[ordered[j]]
		}
		return ordered[i] < ordered[j]
	})

	firstClass := 0
	other := len(m.artistIDs) - assigned // unassigned artists roll to "other"
	var parts []string
	for _, c := range ordered {
		n := inScene[c]
		if n >= clusterMinSize && firstClass < clusterMaxFirstClass {
			firstClass++
			parts = append(parts, fmt.Sprintf("%d in \"Around %s\" (global %d)", n, nameOf(names, anchor[c]), globalSizes[c]))
			continue
		}
		other += n
	}
	fmt.Printf("  %s (%s): roster=%d assigned=%d communities=%d → first-class=%d other=%d\n",
		m.name, m.cbsa, len(m.artistIDs), assigned, len(inScene), firstClass, other)
	for _, p := range parts {
		fmt.Printf("      %s\n", p)
	}
}

// loadArtistNames resolves names for label anchors. Loads all id→name pairs
// for graph nodes in one query (catalog scale: thousands).
func loadArtistNames(database *gorm.DB, nodeSet map[uint]struct{}) map[uint]string {
	ids := make([]uint, 0, len(nodeSet))
	for id := range nodeSet {
		ids = append(ids, id)
	}
	type row struct {
		ID   uint   `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	var rows []row
	if err := database.Table("artists").Select("id, name").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		log.Fatalf("load artist names: %v", err)
	}
	names := make(map[uint]string, len(rows))
	for _, r := range rows {
		names[r.ID] = r.Name
	}
	return names
}

// loadMetroRosters loads every artist keyed to each requested CBSA
// (artists.metro — the scene roster's primary predicate, PSY-1255).
func loadMetroRosters(database *gorm.DB, csv string) []metroRoster {
	var metros []metroRoster
	for _, part := range strings.Split(csv, ",") {
		cbsa := strings.TrimSpace(part)
		if cbsa == "" {
			continue
		}
		display := cbsa
		if p, ok := geo.MetroPrincipalByCBSA(cbsa); ok {
			display = p.City + ", " + p.State
		}
		var ids []uint
		if err := database.Table("artists").Where("metro = ?", cbsa).Pluck("id", &ids).Error; err != nil {
			log.Fatalf("load metro %s roster: %v", cbsa, err)
		}
		if len(ids) == 0 {
			log.Printf("warning: metro %s (%s) has no keyed artists on this DB", cbsa, display)
		}
		metros = append(metros, metroRoster{cbsa: cbsa, name: display, artistIDs: ids})
	}
	return metros
}

func nameOf(names map[uint]string, id uint) string {
	if n, ok := names[id]; ok {
		return n
	}
	return fmt.Sprintf("artist %d", id)
}

func median(sortedDesc []int) int {
	if len(sortedDesc) == 0 {
		return 0
	}
	return sortedDesc[len(sortedDesc)/2]
}

func loadEnv() {
	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			log.Fatalf("load env file %s: %v", envFile, err)
		}
		log.Printf("loaded env from %s", envFile)
		return
	}
	for _, ef := range []string{".env.development", ".env"} {
		if err := godotenv.Load(ef); err == nil {
			log.Printf("loaded env from %s", ef)
			return
		}
	}
	log.Println("no .env loaded; using process environment")
}

func redactDBHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "<unparseable>"
	}
	return u.Host + u.Path
}
