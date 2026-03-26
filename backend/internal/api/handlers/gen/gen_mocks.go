// gen_mocks.go generates function-field mock structs from contracts interfaces.
//
// Usage:
//
//	cd backend && go run ./internal/api/handlers/gen/ > ./internal/api/handlers/handler_unit_mock_helpers_test.go
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// interfaceConfig holds per-interface generation configuration.
type interfaceConfig struct {
	// mockName overrides the generated mock struct name.
	mockName string
	// skip means do not generate this interface mock.
	skip bool
}

// methodDefault holds a custom default return expression for a specific mock method.
type methodDefault struct {
	// body is the full Go code for the method body when the Fn field is nil.
	body string
}

// configs for specific interfaces. Unlisted interfaces use defaults.
var ifaceConfigs = map[string]interfaceConfig{
	"ShowFullServiceInterface":  {skip: true}, // Composite — use sub-interfaces
	"BookmarkServiceInterface":  {skip: true}, // Not used in handler tests
	"FetcherServiceInterface":   {skip: true}, // Not used in handler tests
	"ReminderServiceInterface":  {skip: true}, // Not used in handler tests
	"SchedulerServiceInterface": {skip: true}, // Not used in handler tests
	"EnrichmentWorkerInterface": {skip: true}, // Not used in handler tests
	"AppleAuthServiceInterface": {skip: true}, // Has its own mock in apple_auth_test.go
	"OAuthCompleter":            {skip: true}, // Not a service
}

// Custom method defaults for methods that need non-zero-value defaults.
// Key format: "MockStructName.MethodName"
var customDefaults = map[string]methodDefault{
	"mockShowAdminService.BatchApproveShows": {
		body: `	return &contracts.BatchShowResult{Succeeded: showIDs, Errors: []contracts.BatchShowError{}}, nil`,
	},
	"mockShowAdminService.BatchRejectShows": {
		body: `	return &contracts.BatchShowResult{Succeeded: showIDs, Errors: []contracts.BatchShowError{}}, nil`,
	},
	"mockAttendanceService.GetAttendanceCounts": {
		body: `	return &contracts.AttendanceCountsResponse{ShowID: showID}, nil`,
	},
	"mockAttendanceService.GetBatchAttendanceCounts": {
		body: `	result := make(map[uint]*contracts.AttendanceCountsResponse)
	for _, id := range showIDs {
		result[id] = &contracts.AttendanceCountsResponse{ShowID: id}
	}
	return result, nil`,
	},
	"mockAttendanceService.GetBatchUserAttendance": {
		body: `	return make(map[uint]string), nil`,
	},
	"mockFollowService.GetBatchFollowerCounts": {
		body: `	result := make(map[uint]int64)
	for _, id := range entityIDs {
		result[id] = 0
	}
	return result, nil`,
	},
	"mockFollowService.GetBatchUserFollowing": {
		body: `	return make(map[uint]bool), nil`,
	},
	"mockExtractionService.ExtractCalendarPage": {
		body: `	return &contracts.CalendarExtractionResponse{Success: true, Events: []contracts.CalendarEvent{}}, nil`,
	},
	"mockAdminStatsService.GetRecentActivity": {
		body: `	return &contracts.ActivityFeedResponse{Events: []contracts.ActivityEvent{}}, nil`,
	},
	"mockUserService.GetFavoriteCities": {
		body: `	return []models.FavoriteCity{}, nil`,
	},
	"mockPasswordValidator.ValidatePassword": {
		body: `	return &contracts.PasswordValidationResult{Valid: true}, nil`,
	},
	"mockVenueService.GetVenueGenreProfile": {
		body: `	return []contracts.GenreCount{}, nil`,
	},
	"mockWebAuthnService.StoreChallenge": {
		body: `	return "challenge-id", nil`,
	},
	"mockSceneService.ListScenes": {
		body: `	return []*contracts.SceneListResponse{}, nil`,
	},
	"mockSceneService.GetActiveArtists": {
		body: `	return []*contracts.SceneArtistResponse{}, 0, nil`,
	},
	"mockSceneService.ParseSceneSlug": {
		body: `	return "", "", fmt.Errorf("scene not found for slug: %s", slug)`,
	},
	"mockSceneService.GetSceneGenreDistribution": {
		body: `	return []contracts.GenreCount{}, nil`,
	},
	"mockSceneService.GetGenreDiversityIndex": {
		body: `	return -1, nil`,
	},
	"mockDataQualityService.GetSummary": {
		body: `	return &contracts.DataQualitySummary{Categories: []contracts.DataQualityCategory{}}, nil`,
	},
	"mockChartsService.GetTrendingShows": {
		body: `	return []contracts.TrendingShow{}, nil`,
	},
	"mockChartsService.GetPopularArtists": {
		body: `	return []contracts.PopularArtist{}, nil`,
	},
	"mockChartsService.GetActiveVenues": {
		body: `	return []contracts.ActiveVenue{}, nil`,
	},
	"mockChartsService.GetHotReleases": {
		body: `	return []contracts.HotRelease{}, nil`,
	},
	"mockChartsService.GetChartsOverview": {
		body: `	return &contracts.ChartsOverview{
		TrendingShows:  []contracts.TrendingShow{},
		PopularArtists: []contracts.PopularArtist{},
		ActiveVenues:   []contracts.ActiveVenue{},
		HotReleases:    []contracts.HotRelease{},
	}, nil`,
	},
	"mockAnalyticsService.GetGrowthMetrics": {
		body: `	return &contracts.GrowthMetricsResponse{
		Shows:    []contracts.MonthlyCount{},
		Artists:  []contracts.MonthlyCount{},
		Venues:   []contracts.MonthlyCount{},
		Releases: []contracts.MonthlyCount{},
		Labels:   []contracts.MonthlyCount{},
		Users:    []contracts.MonthlyCount{},
	}, nil`,
	},
	"mockAnalyticsService.GetEngagementMetrics": {
		body: `	return &contracts.EngagementMetricsResponse{
		Bookmarks:       []contracts.EngagementMetric{},
		TagsAdded:       []contracts.EngagementMetric{},
		TagVotes:        []contracts.EngagementMetric{},
		CollectionItems: []contracts.EngagementMetric{},
		Requests:        []contracts.EngagementMetric{},
		RequestVotes:    []contracts.EngagementMetric{},
		Revisions:       []contracts.EngagementMetric{},
		Follows:         []contracts.EngagementMetric{},
		Attendance:      []contracts.EngagementMetric{},
	}, nil`,
	},
	"mockAnalyticsService.GetCommunityHealth": {
		body: `	return &contracts.CommunityHealthResponse{
		ContributionsPerWeek: []contracts.WeeklyContributions{},
		TopContributors:      []contracts.TopContributor{},
	}, nil`,
	},
	"mockAnalyticsService.GetDataQualityTrends": {
		body: `	return &contracts.DataQualityTrendsResponse{
		ShowsApproved: []contracts.MonthlyCount{},
		ShowsRejected: []contracts.MonthlyCount{},
	}, nil`,
	},
	"mockPipelineService.ExtractVenue": {
		body: `	return &contracts.PipelineResult{
		VenueID:         venueID,
		VenueName:       "Test Venue",
		RenderMethod:    "static",
		EventsExtracted: 5,
		EventsImported:  3,
		DurationMs:      1234,
		DryRun:          dryRun,
	}, nil`,
	},
	"mockVenueSourceConfigService.CreateOrUpdate": {
		body: `	return config, nil`,
	},
	"mockVenueSourceConfigService.GetRejectionStats": {
		body: `	return &contracts.VenueRejectionStats{RejectionBreakdown: make(map[string]int64)}, nil`,
	},
	"mockEnrichmentService.EnrichShow": {
		body: `	return &contracts.EnrichmentResult{ShowID: showID, CompletedSteps: []string{"artist_match", "musicbrainz", "api_crossref"}}, nil`,
	},
	"mockEnrichmentService.GetQueueStats": {
		body: `	return &contracts.EnrichmentQueueStats{}, nil`,
	},
	"mockRequestService.CreateRequest": {
		body: `	desc := description
	return &models.Request{
		ID:          1,
		Title:       title,
		Description: &desc,
		EntityType:  entityType,
		Status:      models.RequestStatusPending,
		RequesterID: userID,
	}, nil`,
	},
	"mockRequestService.GetRequest": {
		body: `	return &models.Request{
		ID:          requestID,
		Title:       "Test Request",
		EntityType:  "artist",
		Status:      models.RequestStatusPending,
		RequesterID: 1,
	}, nil`,
	},
	"mockRequestService.ListRequests": {
		body: `	return []models.Request{
		{ID: 1, Title: "Request 1", EntityType: "artist", Status: models.RequestStatusPending, RequesterID: 1},
	}, 1, nil`,
	},
	"mockRequestService.UpdateRequest": {
		body: `	t := "Updated"
	return &models.Request{ID: requestID, Title: t, EntityType: "artist", Status: models.RequestStatusPending, RequesterID: userID}, nil`,
	},
}

// contractsTypes is the set of type names defined in the contracts package.
// Populated at parse time.
var contractsTypes = map[string]bool{}

func main() {
	contractsDir := filepath.Join("internal", "services", "contracts")

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, contractsDir, nil, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing contracts: %v\n", err)
		os.Exit(1)
	}

	pkg, ok := pkgs["contracts"]
	if !ok {
		fmt.Fprintf(os.Stderr, "Package 'contracts' not found in %s\n", contractsDir)
		os.Exit(1)
	}

	// First pass: collect all type names defined in contracts package
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				contractsTypes[typeSpec.Name.Name] = true
			}
		}
	}

	// Second pass: extract interfaces
	var ifaces []ifaceData

	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				ifaceType, ok := typeSpec.Type.(*ast.InterfaceType)
				if !ok {
					continue
				}
				name := typeSpec.Name.Name
				if !strings.HasSuffix(name, "Interface") {
					continue
				}
				cfg, hasCfg := ifaceConfigs[name]
				if hasCfg && cfg.skip {
					continue
				}

				mockName := "mock" + strings.TrimSuffix(name, "Interface")
				if hasCfg && cfg.mockName != "" {
					mockName = cfg.mockName
				}

				var methods []methodData
				for _, method := range ifaceType.Methods.List {
					if len(method.Names) == 0 {
						continue // Embedded interface
					}
					funcType, ok := method.Type.(*ast.FuncType)
					if !ok {
						continue
					}

					mName := method.Names[0].Name
					params := extractParams(funcType.Params)
					results := extractResults(funcType.Results)

					isVariadic := false
					if funcType.Params != nil && len(funcType.Params.List) > 0 {
						lastField := funcType.Params.List[len(funcType.Params.List)-1]
						if _, ok := lastField.Type.(*ast.Ellipsis); ok {
							isVariadic = true
						}
					}

					methods = append(methods, methodData{
						Name:       mName,
						Params:     params,
						Results:    results,
						IsVariadic: isVariadic,
					})
				}

				ifaces = append(ifaces, ifaceData{
					InterfaceName: name,
					MockName:      mockName,
					Methods:       methods,
				})
			}
		}
	}

	// Sort by interface name for stable output
	sort.Slice(ifaces, func(i, j int) bool {
		return ifaces[i].InterfaceName < ifaces[j].InterfaceName
	})

	// Generate output
	generateFile(ifaces)
}

type ifaceData struct {
	InterfaceName string
	MockName      string
	Methods       []methodData
}

type methodData struct {
	Name       string
	Params     []paramData
	Results    []resultData
	IsVariadic bool
}

type paramData struct {
	Name string
	Type string
}

type resultData struct {
	Name string
	Type string
}

func extractParams(fields *ast.FieldList) []paramData {
	if fields == nil {
		return nil
	}
	var params []paramData
	counter := 0
	for _, field := range fields.List {
		typStr := typeString(field.Type)
		if len(field.Names) == 0 {
			name := inferParamName(typStr, counter)
			counter++
			params = append(params, paramData{Name: name, Type: typStr})
		} else {
			for _, ident := range field.Names {
				params = append(params, paramData{Name: ident.Name, Type: typStr})
				counter++
			}
		}
	}
	return params
}

func extractResults(fields *ast.FieldList) []resultData {
	if fields == nil {
		return nil
	}
	var results []resultData
	for _, field := range fields.List {
		typStr := typeString(field.Type)
		if len(field.Names) == 0 {
			results = append(results, resultData{Type: typStr})
		} else {
			for _, ident := range field.Names {
				results = append(results, resultData{Name: ident.Name, Type: typStr})
			}
		}
	}
	return results
}

// typeString converts an AST type expression to a Go type string,
// adding "contracts." prefix for types defined in the contracts package.
func typeString(node ast.Node) string {
	switch n := node.(type) {
	case *ast.Ident:
		// Check if it's a contracts type that needs prefixing
		if contractsTypes[n.Name] {
			return "contracts." + n.Name
		}
		return n.Name
	case *ast.SelectorExpr:
		// Already qualified (e.g., models.User, goth.User)
		return typeString(n.X) + "." + n.Sel.Name
	case *ast.StarExpr:
		return "*" + typeString(n.X)
	case *ast.ArrayType:
		return "[]" + typeString(n.Elt)
	case *ast.MapType:
		return "map[" + typeString(n.Key) + "]" + typeString(n.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.Ellipsis:
		return "..." + typeString(n.Elt)
	case *ast.FuncType:
		var buf strings.Builder
		buf.WriteString("func(")
		if n.Params != nil {
			for i, field := range n.Params.List {
				if i > 0 {
					buf.WriteString(", ")
				}
				buf.WriteString(typeString(field.Type))
			}
		}
		buf.WriteString(")")
		if n.Results != nil && len(n.Results.List) > 0 {
			buf.WriteString(" ")
			if len(n.Results.List) == 1 && len(n.Results.List[0].Names) == 0 {
				buf.WriteString(typeString(n.Results.List[0].Type))
			} else {
				buf.WriteString("(")
				for i, field := range n.Results.List {
					if i > 0 {
						buf.WriteString(", ")
					}
					buf.WriteString(typeString(field.Type))
				}
				buf.WriteString(")")
			}
		}
		return buf.String()
	default:
		return "interface{}"
	}
}

// inferParamName generates a reasonable parameter name from the type.
func inferParamName(typStr string, index int) string {
	// Strip prefixes
	base := typStr
	base = strings.TrimPrefix(base, "*")
	base = strings.TrimPrefix(base, "[]")
	base = strings.TrimPrefix(base, "contracts.")
	base = strings.TrimPrefix(base, "models.")
	if strings.HasPrefix(base, "map[") {
		return fmt.Sprintf("arg%d", index)
	}
	if strings.HasPrefix(base, "...") {
		base = strings.TrimPrefix(base, "...")
		base = strings.TrimPrefix(base, "contracts.")
		base = strings.TrimPrefix(base, "models.")
	}

	// Get last part after dot (for any remaining qualifiers)
	if idx := strings.LastIndex(base, "."); idx >= 0 {
		base = base[idx+1:]
	}

	if len(base) == 0 {
		return fmt.Sprintf("arg%d", index)
	}

	// lowercaseFirst
	runes := []rune(base)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func generateFile(ifaces []ifaceData) {
	fmt.Println(`// Code generated by gen/gen_mocks.go; DO NOT EDIT.
// To regenerate:
//   cd backend && go run ./internal/api/handlers/gen/ > ./internal/api/handlers/handler_unit_mock_helpers_test.go

//go:generate go run ./gen/ > handler_unit_mock_helpers_test.go

package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/markbates/goth"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// Suppress unused import warnings.
var (
	_ context.Context
	_ fmt.Stringer
	_ http.ResponseWriter
	_ time.Duration
	_ *protocol.CredentialCreation
	_ *webauthn.SessionData
	_ goth.User
	_ *gorm.DB
)`)

	for _, iface := range ifaces {
		fmt.Printf("\n// ============================================================================\n")
		fmt.Printf("// Mock: %s\n", iface.InterfaceName)
		fmt.Printf("// ============================================================================\n\n")

		// Struct definition with function fields
		fmt.Printf("type %s struct {\n", iface.MockName)
		for _, m := range iface.Methods {
			fnFieldName := toLowerFirst(m.Name) + "Fn"
			fnType := formatFuncType(m.Params, m.Results, m.IsVariadic)
			fmt.Printf("\t%s %s\n", fnFieldName, fnType)
		}
		fmt.Println("}")
		fmt.Println()

		// Method implementations
		for _, m := range iface.Methods {
			fnFieldName := toLowerFirst(m.Name) + "Fn"
			paramList := formatParamList(m.Params)
			paramNames := formatParamNames(m.Params, m.IsVariadic)

			resultSig := ""
			if len(m.Results) > 0 {
				resultSig = " (" + formatResultTypes(m.Results) + ")"
			}

			fmt.Printf("func (m *%s) %s(%s)%s {\n", iface.MockName, m.Name, paramList, resultSig)
			fmt.Printf("\tif m.%s != nil {\n", fnFieldName)
			if len(m.Results) > 0 {
				fmt.Printf("\t\treturn m.%s(%s)\n", fnFieldName, paramNames)
			} else {
				fmt.Printf("\t\tm.%s(%s)\n", fnFieldName, paramNames)
			}
			fmt.Println("\t}")

			// Default return
			key := iface.MockName + "." + m.Name
			if custom, ok := customDefaults[key]; ok {
				fmt.Println(custom.body)
			} else if len(m.Results) > 0 {
				fmt.Printf("\treturn %s\n", formatZeroValues(m.Results))
			}

			fmt.Println("}")
		}
	}

	// Compile-time interface checks
	fmt.Println()
	fmt.Println("// ============================================================================")
	fmt.Println("// Compile-time interface satisfaction checks")
	fmt.Println("// ============================================================================")
	fmt.Println()
	for _, iface := range ifaces {
		fmt.Printf("var _ contracts.%s = (*%s)(nil)\n", iface.InterfaceName, iface.MockName)
	}
}

func toLowerFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func formatFuncType(params []paramData, results []resultData, isVariadic bool) string {
	var buf strings.Builder
	buf.WriteString("func(")
	for i, p := range params {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(p.Type)
	}
	buf.WriteString(")")
	if len(results) > 0 {
		buf.WriteString(" (")
		for i, r := range results {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(r.Type)
		}
		buf.WriteString(")")
	}
	return buf.String()
}

func formatParamList(params []paramData) string {
	var parts []string
	for _, p := range params {
		parts = append(parts, p.Name+" "+p.Type)
	}
	return strings.Join(parts, ", ")
}

func formatParamNames(params []paramData, isVariadic bool) string {
	var parts []string
	for i, p := range params {
		name := p.Name
		if isVariadic && i == len(params)-1 {
			name += "..."
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, ", ")
}

func formatResultTypes(results []resultData) string {
	var parts []string
	for _, r := range results {
		parts = append(parts, r.Type)
	}
	return strings.Join(parts, ", ")
}

func formatZeroValues(results []resultData) string {
	var parts []string
	for _, r := range results {
		parts = append(parts, zeroValue(r.Type))
	}
	return strings.Join(parts, ", ")
}

func zeroValue(typ string) string {
	switch typ {
	case "error":
		return "nil"
	case "bool":
		return "false"
	case "int", "int8", "int16", "int32", "int64":
		return "0"
	case "uint", "uint8", "uint16", "uint32", "uint64":
		return "0"
	case "float32", "float64":
		return "0"
	case "string":
		return `""`
	case "time.Duration":
		return "0"
	case "[]byte":
		return "nil"
	default:
		if strings.HasPrefix(typ, "*") || strings.HasPrefix(typ, "[]") || strings.HasPrefix(typ, "map[") {
			return "nil"
		}
		return typ + "{}"
	}
}
