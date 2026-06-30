// Command disparity-analysis is a READ-ONLY tuning tool for the radio-co-occurrence disparity-filter
// backbone (PSY-1261). It loads radio_artist_affinity, computes each edge's disparity significance
// (catalog.DisparitySignificance), and reports — across a sweep of alpha thresholds — the backbone
// size, niche-link survival, and a comparison against a naive global weight cutoff. It NEVER writes.
// Use it to choose + justify the alpha the precompute job stores against (the AC evidence).
//
// Meaningful output requires SCENE-SCALE data: the local dev DB's radio graph is near-empty, so
// run it against stage (or prod) — pull the read-only DATABASE_PUBLIC_URL into a temp env (see the
// stage-ops convention) and pass it via --env. The tool only SELECTs; it never writes.
//
// Usage:
//
//	go run ./cmd/disparity-analysis --env /tmp/stage.env            # stage (read-only)
//	go run ./cmd/disparity-analysis --env /tmp/stage.env --ego cola # + ego spot-check
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
)

var (
	envFile  string
	egoSlug  string
	alphaCSV string
)

// defaultAlphas is the default set of significance thresholds to report.
var defaultAlphas = []float64{0.01, 0.05, 0.10, 0.20, 0.50}

func main() {
	flag.StringVar(&envFile, "env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.StringVar(&egoSlug, "ego", "", "Optional artist slug to spot-check one ego graph (e.g. cola)")
	flag.StringVar(&alphaCSV, "alphas", "", "Comma-separated alpha sweep (default 0.01,0.05,0.1,0.2,0.5)")
	flag.Parse()

	loadEnv()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("connect db: %v", err)
	}
	database := db.GetDB()

	fmt.Println("=== Disparity-Filter Backbone Analysis (READ-ONLY) — PSY-1261 ===")
	fmt.Printf("Target: db=%s\n\n", redactDBHost(cfg.Database.URL))

	// Load the weighted radio graph. Weight = co_occurrence_count (the raw tie strength; the
	// disparity filter normalizes per-node, so absolute scale is irrelevant — only the per-node
	// fraction matters). Pairs are already canonically ordered (artist_a_id < artist_b_id).
	type affinityRow struct {
		ArtistAID         uint
		ArtistBID         uint
		CoOccurrenceCount int
	}
	var rows []affinityRow
	if err := database.
		Table("radio_artist_affinity").
		Select("artist_a_id, artist_b_id, co_occurrence_count").
		Find(&rows).Error; err != nil {
		log.Fatalf("load radio_artist_affinity: %v", err)
	}
	if len(rows) == 0 {
		log.Fatalf("radio_artist_affinity is empty — nothing to analyze (seed/affinity not computed on this DB)")
	}

	edges := make([]catalogsvc.WeightedEdge, 0, len(rows))
	for _, r := range rows {
		edges = append(edges, catalogsvc.WeightedEdge{A: r.ArtistAID, B: r.ArtistBID, Weight: float64(r.CoOccurrenceCount)})
	}
	sigByEdge := catalogsvc.DisparitySignificance(edges)

	// Per-node degree, for the degree distribution + niche bucketing.
	degree := map[uint]int{}
	for _, e := range edges {
		degree[e.A]++
		degree[e.B]++
	}

	alphas := parseAlphas(alphaCSV)

	printGraphSummary(len(degree), len(edges), degree)
	printBackboneSweep(edges, sigByEdge, degree, alphas)
	printNicheSurvival(edges, sigByEdge, degree, alphas)
	printNaiveThresholdComparison(edges, sigByEdge, degree, alphas)

	if egoSlug != "" {
		printEgoSpotCheck(database, egoSlug, edges, sigByEdge, alphas)
	} else {
		fmt.Println("(pass --ego <slug> to spot-check a single ego graph, e.g. --ego cola)")
	}
}

// nicheDegreeMax is the degree at/below which a node counts as "niche" for the survival report —
// the low-connectivity artists whose few links a global weight cutoff would erase. It's a
// hand-picked REPORTING bucket (not a model parameter — the disparity filter itself has no degree
// cutoff), chosen to spotlight the long tail; tune it freely to slice the survival numbers
// differently. The headline niche-preservation claim (degree-1 links always survive) is independent
// of this value.
const nicheDegreeMax = 3

func printGraphSummary(nodes, edges int, degree map[uint]int) {
	var d1, d2to5, d6to20, d21plus int
	for _, k := range degree {
		switch {
		case k <= 1:
			d1++
		case k <= 5:
			d2to5++
		case k <= 20:
			d6to20++
		default:
			d21plus++
		}
	}
	fmt.Printf("Radio co-occurrence graph: %d nodes, %d edges (weight = co_occurrence_count)\n", nodes, edges)
	fmt.Printf("Degree distribution: deg1=%d  deg2-5=%d  deg6-20=%d  deg21+=%d\n\n", d1, d2to5, d6to20, d21plus)
}

func printBackboneSweep(edges []catalogsvc.WeightedEdge, sigByEdge map[catalogsvc.EdgeKey]float64, degree map[uint]int, alphas []float64) {
	fmt.Println("--- Backbone size by alpha (disparity filter: keep edge iff significance < alpha) ---")
	fmt.Printf("%-8s %12s %8s %12s\n", "alpha", "edges kept", "% kept", "isolated")
	total := len(edges)
	for _, a := range alphas {
		kept, connected := backboneStats(edges, sigByEdge, a)
		isolated := len(degree) - connected
		fmt.Printf("%-8.2f %12d %7.1f%% %12d\n", a, kept, pct(kept, total), isolated)
	}
	fmt.Println()
}

func printNicheSurvival(edges []catalogsvc.WeightedEdge, sigByEdge map[catalogsvc.EdgeKey]float64, degree map[uint]int, alphas []float64) {
	fmt.Printf("--- Niche-link survival (edges touching a node with degree <= %d) ---\n", nicheDegreeMax)
	fmt.Printf("%-8s %18s %8s\n", "alpha", "niche kept/total", "% kept")
	nicheTotal := 0
	for _, e := range edges {
		if degree[e.A] <= nicheDegreeMax || degree[e.B] <= nicheDegreeMax {
			nicheTotal++
		}
	}
	for _, a := range alphas {
		kept := 0
		for _, e := range edges {
			if degree[e.A] > nicheDegreeMax && degree[e.B] > nicheDegreeMax {
				continue
			}
			if sigByEdge[canonical(e)] < a {
				kept++
			}
		}
		fmt.Printf("%-8.2f %18s %7.1f%%\n", a, fmt.Sprintf("%d/%d", kept, nicheTotal), pct(kept, nicheTotal))
	}
	fmt.Println()
}

// printNaiveThresholdComparison contrasts the disparity backbone with a naive "keep the K
// globally-heaviest edges" cutoff sized to the SAME edge count, to show (AC #2) that the global
// cutoff discards niche links the disparity filter preserves.
func printNaiveThresholdComparison(edges []catalogsvc.WeightedEdge, sigByEdge map[catalogsvc.EdgeKey]float64, degree map[uint]int, alphas []float64) {
	fmt.Println("--- Disparity backbone vs a naive global top-K-by-weight cutoff of the SAME size ---")
	fmt.Printf("%-8s %12s %22s %22s\n", "alpha", "backbone K", "disparity niche kept", "global-cutoff niche kept")

	// Edges sorted by weight desc, for the global top-K cutoff.
	byWeightDesc := make([]catalogsvc.WeightedEdge, len(edges))
	copy(byWeightDesc, edges)
	sort.Slice(byWeightDesc, func(i, j int) bool { return byWeightDesc[i].Weight > byWeightDesc[j].Weight })

	isNiche := func(e catalogsvc.WeightedEdge) bool {
		return degree[e.A] <= nicheDegreeMax || degree[e.B] <= nicheDegreeMax
	}
	nicheTotal := 0
	for _, e := range edges {
		if isNiche(e) {
			nicheTotal++
		}
	}

	for _, a := range alphas {
		k, _ := backboneStats(edges, sigByEdge, a)

		dispNiche := 0
		for _, e := range edges {
			if isNiche(e) && sigByEdge[canonical(e)] < a {
				dispNiche++
			}
		}
		globalNiche := 0
		for i := 0; i < k && i < len(byWeightDesc); i++ {
			if isNiche(byWeightDesc[i]) {
				globalNiche++
			}
		}
		fmt.Printf("%-8.2f %12d %21s %21s\n", a, k,
			fmt.Sprintf("%d/%d (%.0f%%)", dispNiche, nicheTotal, pct(dispNiche, nicheTotal)),
			fmt.Sprintf("%d/%d (%.0f%%)", globalNiche, nicheTotal, pct(globalNiche, nicheTotal)))
	}
	fmt.Println()
}

// printEgoSpotCheck reports the legibility of ONE artist's radio ego graph before/after the
// backbone (AC #1's per-ego check) — how many of its direct radio links survive at each alpha.
func printEgoSpotCheck(database *gorm.DB, slug string, edges []catalogsvc.WeightedEdge, sigByEdge map[catalogsvc.EdgeKey]float64, alphas []float64) {
	var artist struct {
		ID   uint
		Name string
	}
	if err := database.Table("artists").Select("id, name").Where("slug = ?", slug).Scan(&artist).Error; err != nil || artist.ID == 0 {
		fmt.Printf("--- Ego spot-check: --ego %q not found (skipped) ---\n\n", slug)
		return
	}

	ego := make([]catalogsvc.WeightedEdge, 0)
	for _, e := range edges {
		if e.A == artist.ID || e.B == artist.ID {
			ego = append(ego, e)
		}
	}
	fmt.Printf("--- Ego spot-check: %s (id %d, %d direct radio links) ---\n", artist.Name, artist.ID, len(ego))
	if len(ego) == 0 {
		fmt.Println("(no radio co-occurrence links for this artist)")
		fmt.Println()
		return
	}
	fmt.Printf("%-8s %12s %8s\n", "alpha", "links kept", "% kept")
	for _, a := range alphas {
		kept := 0
		for _, e := range ego {
			if sigByEdge[canonical(e)] < a {
				kept++
			}
		}
		fmt.Printf("%-8.2f %12s %7.1f%%\n", a, fmt.Sprintf("%d/%d", kept, len(ego)), pct(kept, len(ego)))
	}
	fmt.Println()
}

func backboneStats(edges []catalogsvc.WeightedEdge, sigByEdge map[catalogsvc.EdgeKey]float64, alpha float64) (kept, connectedNodes int) {
	seen := map[uint]struct{}{}
	for _, e := range edges {
		if sigByEdge[canonical(e)] < alpha {
			kept++
			seen[e.A] = struct{}{}
			seen[e.B] = struct{}{}
		}
	}
	return kept, len(seen)
}

func canonical(e catalogsvc.WeightedEdge) catalogsvc.EdgeKey {
	if e.A > e.B {
		return catalogsvc.EdgeKey{e.B, e.A}
	}
	return catalogsvc.EdgeKey{e.A, e.B}
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(n) / float64(total)
}

func parseAlphas(csv string) []float64 {
	if csv == "" {
		return defaultAlphas
	}
	var out []float64
	for _, part := range strings.Split(csv, ",") {
		if v, err := strconv.ParseFloat(strings.TrimSpace(part), 64); err == nil && v > 0 {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return defaultAlphas
	}
	return out
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
