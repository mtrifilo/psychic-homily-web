// Package main implements a scaffolding CLI tool that generates boilerplate files
// for adding a new entity to the Psychic Homily codebase.
//
// Usage:
//
//	go run ./cmd/scaffold <entity-name> [--fields "name:string,description:text,url:string,is_active:bool"]
//
// Example:
//
//	go run ./cmd/scaffold promoter --fields "name:string,website:url,is_active:bool"
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"unicode"
)

// FieldDef represents a parsed field definition
type FieldDef struct {
	Name       string // e.g. "website"
	GoName     string // e.g. "Website"
	Type       string // raw type key: string, text, bool, int, url, date, jsonb
	SQLType    string // e.g. "VARCHAR(500)"
	SQLDefault string // e.g. "DEFAULT false"
	GoType     string // e.g. "string"
	GoPointer  bool   // whether to use *type in model
	TSType     string // e.g. "string"
	JSONTag    string // e.g. "website"
	SnakeCase  string // e.g. "website"
	GormTag    string // full gorm struct tag value
	IsRequired bool   // whether the field is non-nullable
}

// EntityDef holds all the computed names for an entity
type EntityDef struct {
	// Names
	Name            string // e.g. "promoter"
	NameTitle       string // e.g. "Promoter"
	NamePlural      string // e.g. "promoters"
	NamePluralTitle string // e.g. "Promoters"
	NameCamel       string // e.g. "promoter"
	NameSnake       string // e.g. "promoter"
	NameKebab       string // e.g. "promoter"

	// Fields
	Fields    []FieldDef
	NameField *FieldDef // the "name" field if present

	// Migration
	MigrationNum string // e.g. "000058"
}

func main() {
	// Rearrange os.Args so the positional entity name comes after all flags.
	// Go's flag package stops at the first non-flag argument, so we need to
	// move it to the end to allow flags in any order:
	//   go run ./cmd/scaffold promoter --fields "..." --dry-run
	//   go run ./cmd/scaffold --fields "..." promoter
	// Both work after rearrangement.
	rearrangeArgs()

	fieldsFlag := flag.String("fields", "", `Comma-separated field definitions (e.g. "name:string,description:text,url:string,is_active:bool")`)
	dryRun := flag.Bool("dry-run", false, "Print generated files to stdout instead of writing to disk")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: go run ./cmd/scaffold <entity-name> [--fields \"name:string,...\"] [--dry-run]\n\n")
		fmt.Fprintf(os.Stderr, "Generates boilerplate files for a new entity.\n\n")
		fmt.Fprintf(os.Stderr, "Field types:\n")
		fmt.Fprintf(os.Stderr, "  string  → VARCHAR(255) / string / string\n")
		fmt.Fprintf(os.Stderr, "  text    → TEXT / string / string\n")
		fmt.Fprintf(os.Stderr, "  bool    → BOOLEAN DEFAULT false / bool / boolean\n")
		fmt.Fprintf(os.Stderr, "  int     → INTEGER / int / number\n")
		fmt.Fprintf(os.Stderr, "  url     → VARCHAR(500) / string / string\n")
		fmt.Fprintf(os.Stderr, "  date    → DATE / *time.Time / string\n")
		fmt.Fprintf(os.Stderr, "  jsonb   → JSONB / *json.RawMessage / Record<string, unknown>\n\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	entityName := strings.ToLower(flag.Arg(0))
	if !isValidIdentifier(entityName) {
		fmt.Fprintf(os.Stderr, "Error: entity name %q is not a valid identifier (use lowercase letters and underscores)\n", entityName)
		os.Exit(1)
	}

	// Parse fields
	fields, err := parseFields(*fieldsFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing fields: %v\n", err)
		os.Exit(1)
	}

	// Detect project root
	projectRoot, err := detectProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Detect next migration number
	migNum, err := detectNextMigrationNumber(filepath.Join(projectRoot, "backend", "db", "migrations"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error detecting migration number: %v\n", err)
		os.Exit(1)
	}

	// Build entity definition
	entity := buildEntityDef(entityName, fields, migNum)

	// Generate all files
	generatedFiles, err := generateFiles(entity, projectRoot, *dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating files: %v\n", err)
		os.Exit(1)
	}

	// Print summary
	fmt.Println()
	fmt.Println("=== Scaffolding complete ===")
	fmt.Println()
	fmt.Println("Generated files:")
	for _, f := range generatedFiles {
		rel, _ := filepath.Rel(projectRoot, f)
		fmt.Printf("  %s\n", rel)
	}

	// Print wiring instructions
	printWiringInstructions(entity)
}

// rearrangeArgs moves the positional entity-name argument after all flags
// so that Go's flag package can parse flags regardless of argument order.
func rearrangeArgs() {
	args := os.Args[1:] // skip program name
	var flags []string
	var positional []string

	i := 0
	for i < len(args) {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			// If this flag takes a value (not a bool flag like --dry-run),
			// the next arg is the value
			if (arg == "--fields" || arg == "-fields") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else {
			positional = append(positional, arg)
		}
		i++
	}

	// Rebuild os.Args: program name + flags + positional
	newArgs := []string{os.Args[0]}
	newArgs = append(newArgs, flags...)
	newArgs = append(newArgs, positional...)
	os.Args = newArgs
}

// isValidIdentifier checks that the name is a valid Go/SQL identifier
func isValidIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 && !unicode.IsLetter(r) {
			return false
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

// parseFields parses "name:string,description:text" into FieldDef slices
func parseFields(raw string) ([]FieldDef, error) {
	if raw == "" {
		return nil, nil
	}

	var fields []FieldDef
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid field definition %q (expected name:type)", part)
		}
		name := strings.TrimSpace(kv[0])
		typeName := strings.TrimSpace(kv[1])

		fd, err := buildFieldDef(name, typeName)
		if err != nil {
			return nil, err
		}
		fields = append(fields, fd)
	}
	return fields, nil
}

// buildFieldDef converts a field name and type into a FieldDef
func buildFieldDef(name, typeName string) (FieldDef, error) {
	fd := FieldDef{
		Name:      name,
		GoName:    toGoName(name),
		SnakeCase: name,
		JSONTag:   name,
		Type:      typeName,
	}

	// "name" field is always required
	if name == "name" {
		fd.IsRequired = true
	}

	switch typeName {
	case "string":
		fd.SQLType = "VARCHAR(255)"
		fd.GoType = "string"
		fd.GoPointer = name != "name" // name is required, others are optional
		fd.TSType = "string"
		if name == "name" {
			fd.GormTag = fmt.Sprintf(`gorm:"column:%s;not null"`, name)
		} else {
			fd.GormTag = fmt.Sprintf(`gorm:"column:%s;size:255"`, name)
		}
	case "text":
		fd.SQLType = "TEXT"
		fd.GoType = "string"
		fd.GoPointer = true
		fd.TSType = "string"
		fd.GormTag = fmt.Sprintf(`gorm:"column:%s;type:text"`, name)
	case "bool":
		fd.SQLType = "BOOLEAN"
		fd.SQLDefault = "DEFAULT false"
		fd.GoType = "bool"
		fd.GoPointer = false
		fd.TSType = "boolean"
		fd.GormTag = fmt.Sprintf(`gorm:"column:%s;not null;default:false"`, name)
	case "int":
		fd.SQLType = "INTEGER"
		fd.GoType = "int"
		fd.GoPointer = true
		fd.TSType = "number"
		fd.GormTag = fmt.Sprintf(`gorm:"column:%s"`, name)
	case "url":
		fd.SQLType = "VARCHAR(500)"
		fd.GoType = "string"
		fd.GoPointer = true
		fd.TSType = "string"
		fd.GormTag = fmt.Sprintf(`gorm:"column:%s;size:500"`, name)
	case "date":
		fd.SQLType = "DATE"
		fd.GoType = "time.Time"
		fd.GoPointer = true
		fd.TSType = "string"
		fd.GormTag = fmt.Sprintf(`gorm:"column:%s;type:date"`, name)
	case "jsonb":
		fd.SQLType = "JSONB"
		fd.SQLDefault = "DEFAULT '{}'"
		fd.GoType = "json.RawMessage"
		fd.GoPointer = true
		fd.TSType = "Record<string, unknown>"
		fd.GormTag = fmt.Sprintf(`gorm:"column:%s;type:jsonb;default:'{}'"`, name)
	default:
		return FieldDef{}, fmt.Errorf("unknown field type %q (valid: string, text, bool, int, url, date, jsonb)", typeName)
	}

	return fd, nil
}

// toGoName converts snake_case to PascalCase
func toGoName(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "id" {
			parts[i] = "ID"
		} else if p == "url" {
			parts[i] = "URL"
		} else if p == "api" {
			parts[i] = "API"
		} else if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

// toCamelCase converts snake_case to camelCase
func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if i == 0 {
			parts[i] = strings.ToLower(p)
		} else if p == "id" {
			parts[i] = "Id"
		} else if p == "url" {
			parts[i] = "Url"
		} else if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

// pluralize applies simple English pluralization rules
func pluralize(s string) string {
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "x") ||
		strings.HasSuffix(s, "sh") || strings.HasSuffix(s, "ch") {
		return s + "es"
	}
	if strings.HasSuffix(s, "y") && len(s) > 1 {
		c := s[len(s)-2]
		if c != 'a' && c != 'e' && c != 'i' && c != 'o' && c != 'u' {
			return s[:len(s)-1] + "ies"
		}
	}
	return s + "s"
}

// toKebabCase converts snake_case to kebab-case
func toKebabCase(s string) string {
	return strings.ReplaceAll(s, "_", "-")
}

// buildEntityDef constructs the full entity definition
func buildEntityDef(name string, fields []FieldDef, migNum string) EntityDef {
	plural := pluralize(name)
	titleName := toGoName(name)
	titlePlural := toGoName(plural)

	entity := EntityDef{
		Name:            name,
		NameTitle:       titleName,
		NamePlural:      plural,
		NamePluralTitle: titlePlural,
		NameCamel:       toCamelCase(name),
		NameSnake:       name,
		NameKebab:       toKebabCase(name),
		Fields:          fields,
		MigrationNum:    migNum,
	}

	// Find the name field if present
	for i := range fields {
		if fields[i].Name == "name" {
			entity.NameField = &fields[i]
			break
		}
	}

	return entity
}

// detectProjectRoot finds the project root by looking for go.mod in the backend dir
func detectProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		// Check if we're in the project root (has backend/ and frontend/)
		if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "frontend", "package.json")); err == nil {
				return dir, nil
			}
		}
		// Check if we're in the backend directory
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			parent := filepath.Dir(dir)
			if _, err := os.Stat(filepath.Join(parent, "frontend", "package.json")); err == nil {
				return parent, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("could not find project root (expected backend/go.mod and frontend/package.json)")
}

// detectNextMigrationNumber scans the migrations directory for the highest number
func detectNextMigrationNumber(migrationsDir string) (string, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return "", fmt.Errorf("could not read migrations directory %s: %w", migrationsDir, err)
	}

	maxNum := 0
	for _, entry := range entries {
		name := entry.Name()
		if len(name) >= 6 {
			numStr := name[:6]
			num, err := strconv.Atoi(numStr)
			if err == nil && num > maxNum {
				maxNum = num
			}
		}
	}

	return fmt.Sprintf("%06d", maxNum+1), nil
}

type generatedFile struct {
	path    string
	content string
}

// generateFiles creates all the scaffolded files
func generateFiles(entity EntityDef, projectRoot string, dryRun bool) ([]string, error) {
	files := []generatedFile{}

	// 1. Migration up
	migUp, err := renderTemplate("migration_up", tmplMigrationUp, entity)
	if err != nil {
		return nil, fmt.Errorf("migration up: %w", err)
	}
	files = append(files, generatedFile{
		path:    filepath.Join(projectRoot, "backend", "db", "migrations", fmt.Sprintf("%s_create_%s.up.sql", entity.MigrationNum, entity.NamePlural)),
		content: migUp,
	})

	// 2. Migration down
	migDown, err := renderTemplate("migration_down", tmplMigrationDown, entity)
	if err != nil {
		return nil, fmt.Errorf("migration down: %w", err)
	}
	files = append(files, generatedFile{
		path:    filepath.Join(projectRoot, "backend", "db", "migrations", fmt.Sprintf("%s_create_%s.down.sql", entity.MigrationNum, entity.NamePlural)),
		content: migDown,
	})

	// 3. Model
	model, err := renderTemplate("model", tmplModel, entity)
	if err != nil {
		return nil, fmt.Errorf("model: %w", err)
	}
	files = append(files, generatedFile{
		path:    filepath.Join(projectRoot, "backend", "internal", "models", entity.NameSnake+".go"),
		content: model,
	})

	// 4. Contract types
	contract, err := renderTemplate("contract", tmplContract, entity)
	if err != nil {
		return nil, fmt.Errorf("contract: %w", err)
	}
	files = append(files, generatedFile{
		path:    filepath.Join(projectRoot, "backend", "internal", "services", "contracts", entity.NameSnake+".go"),
		content: contract,
	})

	// 5. Service stub
	service, err := renderTemplate("service", tmplService, entity)
	if err != nil {
		return nil, fmt.Errorf("service: %w", err)
	}
	files = append(files, generatedFile{
		path:    filepath.Join(projectRoot, "backend", "internal", "services", "catalog", entity.NameSnake+"_service.go"),
		content: service,
	})

	// 6. Handler stub
	handler, err := renderTemplate("handler", tmplHandler, entity)
	if err != nil {
		return nil, fmt.Errorf("handler: %w", err)
	}
	files = append(files, generatedFile{
		path:    filepath.Join(projectRoot, "backend", "internal", "api", "handlers", entity.NameSnake+".go"),
		content: handler,
	})

	// 7. Frontend feature module
	featureDir := filepath.Join(projectRoot, "frontend", "features", entity.NamePlural)
	hooksDir := filepath.Join(featureDir, "hooks")

	// types.ts
	featureTypes, err := renderTemplate("feature_types", tmplFeatureTypes, entity)
	if err != nil {
		return nil, fmt.Errorf("feature types: %w", err)
	}
	files = append(files, generatedFile{
		path:    filepath.Join(featureDir, "types.ts"),
		content: featureTypes,
	})

	// api.ts
	featureAPI, err := renderTemplate("feature_api", tmplFeatureAPI, entity)
	if err != nil {
		return nil, fmt.Errorf("feature api: %w", err)
	}
	files = append(files, generatedFile{
		path:    filepath.Join(featureDir, "api.ts"),
		content: featureAPI,
	})

	// hooks/use<Entities>.ts (queries)
	featureHooks, err := renderTemplate("feature_hooks", tmplFeatureHooks, entity)
	if err != nil {
		return nil, fmt.Errorf("feature hooks: %w", err)
	}
	files = append(files, generatedFile{
		path:    filepath.Join(hooksDir, fmt.Sprintf("use%s.ts", entity.NamePluralTitle)),
		content: featureHooks,
	})

	// hooks/useAdmin<Entities>.ts (mutations)
	featureAdminHooks, err := renderTemplate("feature_admin_hooks", tmplFeatureAdminHooks, entity)
	if err != nil {
		return nil, fmt.Errorf("feature admin hooks: %w", err)
	}
	files = append(files, generatedFile{
		path:    filepath.Join(hooksDir, fmt.Sprintf("useAdmin%s.ts", entity.NamePluralTitle)),
		content: featureAdminHooks,
	})

	// hooks/index.ts
	featureHooksIndex, err := renderTemplate("feature_hooks_index", tmplFeatureHooksIndex, entity)
	if err != nil {
		return nil, fmt.Errorf("feature hooks index: %w", err)
	}
	files = append(files, generatedFile{
		path:    filepath.Join(hooksDir, "index.ts"),
		content: featureHooksIndex,
	})

	// index.ts (barrel)
	featureIndex, err := renderTemplate("feature_index", tmplFeatureIndex, entity)
	if err != nil {
		return nil, fmt.Errorf("feature index: %w", err)
	}
	files = append(files, generatedFile{
		path:    filepath.Join(featureDir, "index.ts"),
		content: featureIndex,
	})

	// Write or print
	var paths []string
	for _, f := range files {
		if dryRun {
			fmt.Printf("\n===== %s =====\n", f.path)
			fmt.Println(f.content)
			paths = append(paths, f.path)
		} else {
			dir := filepath.Dir(f.path)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("creating directory %s: %w", dir, err)
			}
			// Check if file already exists
			if _, err := os.Stat(f.path); err == nil {
				return nil, fmt.Errorf("file already exists: %s (refusing to overwrite)", f.path)
			}
			if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil {
				return nil, fmt.Errorf("writing %s: %w", f.path, err)
			}
			paths = append(paths, f.path)
		}
	}

	return paths, nil
}

// needsJSONImport checks if any field uses json.RawMessage
func needsJSONImport(fields []FieldDef) bool {
	for _, f := range fields {
		if f.Type == "jsonb" {
			return true
		}
	}
	return false
}

// needsTimeImport checks if any field uses time.Time
func needsTimeImport(fields []FieldDef) bool {
	for _, f := range fields {
		if f.Type == "date" {
			return true
		}
	}
	return false
}

// goTypeStr returns the Go type string for a field (with pointer if applicable)
func goTypeStr(f FieldDef) string {
	if f.GoPointer {
		return "*" + f.GoType
	}
	return f.GoType
}

// goTypePtrStr always returns the pointer version of the Go type
func goTypePtrStr(f FieldDef) string {
	return "*" + f.GoType
}

// goTypeRespStr returns the Go type for response structs.
// Bool fields always use value types in responses (they have DB defaults).
func goTypeRespStr(f FieldDef) string {
	if f.Type == "bool" {
		return f.GoType
	}
	if f.IsRequired {
		return f.GoType
	}
	return "*" + f.GoType
}

// tsTypeStr returns the TypeScript type for a field
func tsTypeStr(f FieldDef) string {
	return f.TSType
}

// tsTypeNullStr returns the TypeScript type with | null for optional fields
func tsTypeNullStr(f FieldDef) string {
	if f.IsRequired || f.Type == "bool" {
		return f.TSType
	}
	return f.TSType + " | null"
}

// templateFuncs returns the shared template function map
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"lower":           strings.ToLower,
		"upper":           strings.ToUpper,
		"title":           toGoName,
		"camel":           toCamelCase,
		"plural":          pluralize,
		"snake":           func(s string) string { return s },
		"kebab":           toKebabCase,
		"goType":          goTypeStr,
		"goTypePtr":       goTypePtrStr,
		"goTypeResp":      goTypeRespStr,
		"tsType":          tsTypeStr,
		"tsTypeNull":      tsTypeNullStr,
		"needsJSONImport": needsJSONImport,
		"needsTimeImport": needsTimeImport,
	}
}

// renderTemplate renders a named template with the given data
func renderTemplate(name, tmplText string, data interface{}) (string, error) {
	t, err := template.New(name).Funcs(templateFuncs()).Parse(tmplText)
	if err != nil {
		return "", fmt.Errorf("parsing template %s: %w", name, err)
	}

	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template %s: %w", name, err)
	}

	return buf.String(), nil
}

// printWiringInstructions prints the manual wiring steps
func printWiringInstructions(entity EntityDef) {
	fmt.Println()
	fmt.Println("=== Manual wiring steps ===")
	fmt.Println()
	fmt.Println("1. Add interface to contracts/interfaces.go:")
	fmt.Printf("   // %sServiceInterface defines the contract for %s operations.\n", entity.NameTitle, entity.Name)
	fmt.Printf("   type %sServiceInterface interface {\n", entity.NameTitle)
	fmt.Printf("       Create%s(req *Create%sRequest) (*%sDetailResponse, error)\n", entity.NameTitle, entity.NameTitle, entity.NameTitle)
	fmt.Printf("       Get%s(%sID uint) (*%sDetailResponse, error)\n", entity.NameTitle, entity.NameCamel, entity.NameTitle)
	fmt.Printf("       Get%sBySlug(slug string) (*%sDetailResponse, error)\n", entity.NameTitle, entity.NameTitle)
	fmt.Printf("       List%s(filters map[string]interface{}) ([]*%sListResponse, error)\n", entity.NamePluralTitle, entity.NameTitle)
	fmt.Printf("       Search%s(query string) ([]*%sListResponse, error)\n", entity.NamePluralTitle, entity.NameTitle)
	fmt.Printf("       Update%s(%sID uint, req *Update%sRequest) (*%sDetailResponse, error)\n", entity.NameTitle, entity.NameCamel, entity.NameTitle, entity.NameTitle)
	fmt.Printf("       Delete%s(%sID uint) error\n", entity.NameTitle, entity.NameCamel)
	fmt.Println("   }")
	fmt.Println()

	fmt.Println("2. Wire service in services/container.go:")
	fmt.Printf("   %sService *catalog.%sService\n", entity.NameTitle, entity.NameTitle)
	fmt.Println("   // In NewServiceContainer():")
	fmt.Printf("   %sService := catalog.New%sService(db)\n", entity.NameCamel, entity.NameTitle)
	fmt.Println()

	fmt.Println("3. Add compile-time check in services/interfaces.go:")
	fmt.Printf("   var _ contracts.%sServiceInterface = (*catalog.%sService)(nil)\n", entity.NameTitle, entity.NameTitle)
	fmt.Println()

	fmt.Println("4. Register routes in routes/routes.go:")
	fmt.Printf("   %sHandler := handlers.New%sHandler(sc.%sService, sc.AuditLogService)\n", entity.NameCamel, entity.NameTitle, entity.NameTitle)
	fmt.Printf("   // Public routes:\n")
	fmt.Printf("   huma.Register(publicAPI, huma.Operation{\n")
	fmt.Printf("       OperationID: \"list-%s\",\n", entity.NamePlural)
	fmt.Printf("       Method:      http.MethodGet,\n")
	fmt.Printf("       Path:        \"/%s\",\n", entity.NamePlural)
	fmt.Printf("       Summary:     \"List %s\",\n", entity.NamePlural)
	fmt.Printf("   }, %sHandler.List%sHandler)\n", entity.NameCamel, entity.NamePluralTitle)
	fmt.Println()

	fmt.Println("5. Add query keys to frontend/lib/queryClient.ts:")
	fmt.Printf("   %s: {\n", entity.NamePlural)
	fmt.Printf("     all: ['%s'] as const,\n", entity.NamePlural)
	fmt.Printf("     list: (filters?: Record<string, unknown>) => ['%s', 'list', filters] as const,\n", entity.NamePlural)
	fmt.Printf("     detail: (idOrSlug: string | number) => ['%s', 'detail', String(idOrSlug)] as const,\n", entity.NamePlural)
	fmt.Printf("   },\n")
	fmt.Println()

	fmt.Println("6. Add invalidation helper to createInvalidateQueries in frontend/lib/queryClient.ts:")
	fmt.Printf("   %s: () => queryClient.invalidateQueries({ queryKey: ['%s'] }),\n", entity.NamePlural, entity.NamePlural)
	fmt.Println()

	fmt.Println("7. Add to sidebar nav in frontend/components/layout/Sidebar.tsx:")
	fmt.Printf("   { label: '%s', href: '/%s', icon: <IconComponent /> }\n", entity.NamePluralTitle, entity.NamePlural)
	fmt.Println()

	fmt.Println("8. Add to Cmd+K palette in frontend/components/layout/CommandPalette.tsx:")
	fmt.Printf("   { label: '%s', href: '/%s' }\n", entity.NamePluralTitle, entity.NamePlural)
	fmt.Println()

	fmt.Println("9. Run the migration:")
	fmt.Println("   (apply via your migration tooling or manually against the database)")
	fmt.Println()
}

// ============================================================================
// Templates
// ============================================================================

var tmplMigrationUp = `-- Create {{.NamePlural}} table

CREATE TABLE {{.NamePlural}} (
    id BIGSERIAL PRIMARY KEY,
    slug VARCHAR(255) NOT NULL UNIQUE,
{{- range .Fields}}
{{- if eq .Name "name"}}
    {{.SnakeCase}} {{.SQLType}} NOT NULL,
{{- else if ne .SQLDefault ""}}
    {{.SnakeCase}} {{.SQLType}} {{.SQLDefault}},
{{- else}}
    {{.SnakeCase}} {{.SQLType}},
{{- end}}
{{- end}}

    -- Data provenance
    data_source VARCHAR(50),
    source_confidence NUMERIC(3,2),
    last_verified_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_{{.NamePlural}}_slug ON {{.NamePlural}}(slug);
{{- if .NameField}}
CREATE INDEX idx_{{.NamePlural}}_name ON {{.NamePlural}}(name);
{{- end}}
`

var tmplMigrationDown = `DROP TABLE IF EXISTS {{.NamePlural}};
`

var tmplModel = `package models

import (
{{- if or (needsTimeImport .Fields) true}}
	"time"
{{- end}}
{{- if needsJSONImport .Fields}}
	"encoding/json"
{{- end}}
)

// {{.NameTitle}} represents a {{.Name}} entity
type {{.NameTitle}} struct {
	ID   uint   ` + "`" + `gorm:"primaryKey"` + "`" + `
	Slug string ` + "`" + `gorm:"not null;uniqueIndex;size:255"` + "`" + `
{{- range .Fields}}
	{{.GoName}} {{goType .}} ` + "`" + `{{.GormTag}} json:"{{.JSONTag}}"` + "`" + `
{{- end}}

	// Data provenance fields
	DataSource       *string    ` + "`" + `json:"data_source,omitempty" gorm:"column:data_source;size:50"` + "`" + `
	SourceConfidence *float64   ` + "`" + `json:"source_confidence,omitempty" gorm:"column:source_confidence;type:numeric(3,2)"` + "`" + `
	LastVerifiedAt   *time.Time ` + "`" + `json:"last_verified_at,omitempty" gorm:"column:last_verified_at"` + "`" + `

	CreatedAt time.Time ` + "`" + `gorm:"not null"` + "`" + `
	UpdatedAt time.Time ` + "`" + `gorm:"not null"` + "`" + `
}

// TableName specifies the table name for {{.NameTitle}}
func ({{.NameTitle}}) TableName() string {
	return "{{.NamePlural}}"
}
`

var tmplContract = `package contracts

import "time"

// ──────────────────────────────────────────────
// {{.NameTitle}} types
// ──────────────────────────────────────────────

// Create{{.NameTitle}}Request represents the data needed to create a new {{.Name}}
type Create{{.NameTitle}}Request struct {
{{- range .Fields}}
{{- if .IsRequired}}
	{{.GoName}} {{.GoType}} ` + "`" + `json:"{{.JSONTag}}" validate:"required"` + "`" + `
{{- else if eq .Type "bool"}}
	{{.GoName}} {{.GoType}} ` + "`" + `json:"{{.JSONTag}}"` + "`" + `
{{- else}}
	{{.GoName}} {{goTypePtr .}} ` + "`" + `json:"{{.JSONTag}}"` + "`" + `
{{- end}}
{{- end}}
}

// Update{{.NameTitle}}Request represents the data that can be updated on a {{.Name}}
type Update{{.NameTitle}}Request struct {
{{- range .Fields}}
	{{.GoName}} {{goTypePtr .}} ` + "`" + `json:"{{.JSONTag}}"` + "`" + `
{{- end}}
}

// {{.NameTitle}}DetailResponse represents the {{.Name}} data returned to clients
type {{.NameTitle}}DetailResponse struct {
	ID   uint   ` + "`" + `json:"id"` + "`" + `
	Slug string ` + "`" + `json:"slug"` + "`" + `
{{- range .Fields}}
	{{.GoName}} {{goTypeResp .}} ` + "`" + `json:"{{.JSONTag}}"` + "`" + `
{{- end}}
	CreatedAt time.Time ` + "`" + `json:"created_at"` + "`" + `
	UpdatedAt time.Time ` + "`" + `json:"updated_at"` + "`" + `
}

// {{.NameTitle}}ListResponse represents a {{.Name}} in list views
type {{.NameTitle}}ListResponse struct {
	ID   uint   ` + "`" + `json:"id"` + "`" + `
	Slug string ` + "`" + `json:"slug"` + "`" + `
{{- range .Fields}}
	{{.GoName}} {{goTypeResp .}} ` + "`" + `json:"{{.JSONTag}}"` + "`" + `
{{- end}}
}
`

var tmplService = `package catalog

import (
	"fmt"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// {{.NameTitle}}Service handles {{.Name}}-related business logic
type {{.NameTitle}}Service struct {
	db *gorm.DB
}

// New{{.NameTitle}}Service creates a new {{.Name}} service
func New{{.NameTitle}}Service(database *gorm.DB) *{{.NameTitle}}Service {
	if database == nil {
		database = db.GetDB()
	}
	return &{{.NameTitle}}Service{
		db: database,
	}
}

// Create{{.NameTitle}} creates a new {{.Name}}
func (s *{{.NameTitle}}Service) Create{{.NameTitle}}(req *contracts.Create{{.NameTitle}}Request) (*contracts.{{.NameTitle}}DetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Generate unique slug
{{- if .NameField}}
	baseSlug := utils.GenerateArtistSlug(req.Name)
{{- else}}
	baseSlug := "{{.NameSnake}}"
{{- end}}
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&models.{{.NameTitle}}{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	record := &models.{{.NameTitle}}{
		Slug: slug,
{{- range .Fields}}
		{{.GoName}}: req.{{.GoName}},
{{- end}}
	}

	if err := s.db.Create(record).Error; err != nil {
		return nil, fmt.Errorf("failed to create {{.Name}}: %w", err)
	}

	return s.Get{{.NameTitle}}(record.ID)
}

// Get{{.NameTitle}} retrieves a {{.Name}} by ID
func (s *{{.NameTitle}}Service) Get{{.NameTitle}}({{.NameCamel}}ID uint) (*contracts.{{.NameTitle}}DetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var record models.{{.NameTitle}}
	err := s.db.First(&record, {{.NameCamel}}ID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("{{.Name}} not found: %d", {{.NameCamel}}ID)
		}
		return nil, fmt.Errorf("failed to get {{.Name}}: %w", err)
	}

	return s.buildDetailResponse(&record), nil
}

// Get{{.NameTitle}}BySlug retrieves a {{.Name}} by slug
func (s *{{.NameTitle}}Service) Get{{.NameTitle}}BySlug(slug string) (*contracts.{{.NameTitle}}DetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var record models.{{.NameTitle}}
	err := s.db.Where("slug = ?", slug).First(&record).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("{{.Name}} not found: %s", slug)
		}
		return nil, fmt.Errorf("failed to get {{.Name}}: %w", err)
	}

	return s.buildDetailResponse(&record), nil
}

// List{{.NamePluralTitle}} retrieves {{.NamePlural}} with optional filtering
func (s *{{.NameTitle}}Service) List{{.NamePluralTitle}}(filters map[string]interface{}) ([]*contracts.{{.NameTitle}}ListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := s.db.Model(&models.{{.NameTitle}}{})

	// Apply filters
	for key, value := range filters {
		if str, ok := value.(string); ok && str != "" {
			query = query.Where(key+" = ?", str)
		}
	}

{{- if .NameField}}
	query = query.Order("name ASC")
{{- else}}
	query = query.Order("created_at DESC")
{{- end}}

	var records []models.{{.NameTitle}}
	if err := query.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to list {{.NamePlural}}: %w", err)
	}

	responses := make([]*contracts.{{.NameTitle}}ListResponse, len(records))
	for i, record := range records {
		responses[i] = s.buildListResponse(&record)
	}

	return responses, nil
}

// Search{{.NamePluralTitle}} searches for {{.NamePlural}} by name using ILIKE matching
func (s *{{.NameTitle}}Service) Search{{.NamePluralTitle}}(query string) ([]*contracts.{{.NameTitle}}ListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if query == "" {
		return []*contracts.{{.NameTitle}}ListResponse{}, nil
	}

	var records []models.{{.NameTitle}}
	var err error

{{- if .NameField}}
	if len(query) <= 2 {
		err = s.db.
			Where("LOWER(name) LIKE LOWER(?)", query+"%").
			Order("name ASC").
			Limit(20).
			Find(&records).Error
	} else {
		err = s.db.
			Where("name ILIKE ?", "%"+query+"%").
			Order("name ASC").
			Limit(20).
			Find(&records).Error
	}
{{- else}}
	err = s.db.
		Where("slug ILIKE ?", "%"+query+"%").
		Order("created_at DESC").
		Limit(20).
		Find(&records).Error
{{- end}}

	if err != nil {
		return nil, fmt.Errorf("failed to search {{.NamePlural}}: %w", err)
	}

	responses := make([]*contracts.{{.NameTitle}}ListResponse, len(records))
	for i, record := range records {
		responses[i] = s.buildListResponse(&record)
	}

	return responses, nil
}

// Update{{.NameTitle}} updates an existing {{.Name}}
func (s *{{.NameTitle}}Service) Update{{.NameTitle}}({{.NameCamel}}ID uint, req *contracts.Update{{.NameTitle}}Request) (*contracts.{{.NameTitle}}DetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var record models.{{.NameTitle}}
	if err := s.db.First(&record, {{.NameCamel}}ID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("{{.Name}} not found: %d", {{.NameCamel}}ID)
		}
		return nil, fmt.Errorf("failed to get {{.Name}}: %w", err)
	}

	updates := map[string]interface{}{}

{{- range .Fields}}
{{- if eq .Name "name"}}

	if req.{{.GoName}} != nil {
		updates["{{.SnakeCase}}"] = *req.{{.GoName}}
		// Regenerate slug when name changes
		baseSlug := utils.GenerateArtistSlug(*req.{{.GoName}})
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&models.{{$.NameTitle}}{}).Where("slug = ? AND id != ?", candidate, {{$.NameCamel}}ID).Count(&count)
			return count > 0
		})
		updates["slug"] = slug
	}
{{- else}}

	if req.{{.GoName}} != nil {
		updates["{{.SnakeCase}}"] = *req.{{.GoName}}
	}
{{- end}}
{{- end}}

	if len(updates) > 0 {
		if err := s.db.Model(&models.{{.NameTitle}}{}).Where("id = ?", {{.NameCamel}}ID).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("failed to update {{.Name}}: %w", err)
		}
	}

	return s.Get{{.NameTitle}}({{.NameCamel}}ID)
}

// Delete{{.NameTitle}} deletes a {{.Name}}
func (s *{{.NameTitle}}Service) Delete{{.NameTitle}}({{.NameCamel}}ID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var record models.{{.NameTitle}}
	if err := s.db.First(&record, {{.NameCamel}}ID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("{{.Name}} not found: %d", {{.NameCamel}}ID)
		}
		return fmt.Errorf("failed to get {{.Name}}: %w", err)
	}

	if err := s.db.Delete(&record).Error; err != nil {
		return fmt.Errorf("failed to delete {{.Name}}: %w", err)
	}

	return nil
}

// buildDetailResponse converts a {{.NameTitle}} model to a detail response
func (s *{{.NameTitle}}Service) buildDetailResponse(record *models.{{.NameTitle}}) *contracts.{{.NameTitle}}DetailResponse {
	return &contracts.{{.NameTitle}}DetailResponse{
		ID:   record.ID,
		Slug: record.Slug,
{{- range .Fields}}
		{{.GoName}}: record.{{.GoName}},
{{- end}}
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}

// buildListResponse converts a {{.NameTitle}} model to a list response
func (s *{{.NameTitle}}Service) buildListResponse(record *models.{{.NameTitle}}) *contracts.{{.NameTitle}}ListResponse {
	return &contracts.{{.NameTitle}}ListResponse{
		ID:   record.ID,
		Slug: record.Slug,
{{- range .Fields}}
		{{.GoName}}: record.{{.GoName}},
{{- end}}
	}
}
`

var tmplHandler = `package handlers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// {{.NameTitle}}Handler handles {{.Name}}-related HTTP endpoints
type {{.NameTitle}}Handler struct {
	{{.NameCamel}}Service contracts.{{.NameTitle}}ServiceInterface
	auditLogService      contracts.AuditLogServiceInterface
}

// New{{.NameTitle}}Handler creates a new {{.Name}} handler
func New{{.NameTitle}}Handler({{.NameCamel}}Service contracts.{{.NameTitle}}ServiceInterface, auditLogService contracts.AuditLogServiceInterface) *{{.NameTitle}}Handler {
	return &{{.NameTitle}}Handler{
		{{.NameCamel}}Service: {{.NameCamel}}Service,
		auditLogService:      auditLogService,
	}
}

// ============================================================================
// List {{.NamePluralTitle}}
// ============================================================================

// List{{.NamePluralTitle}}Request represents the request for listing {{.NamePlural}}
type List{{.NamePluralTitle}}Request struct {
}

// List{{.NamePluralTitle}}Response represents the response for listing {{.NamePlural}}
type List{{.NamePluralTitle}}Response struct {
	Body struct {
		{{.NamePluralTitle}} []*contracts.{{.NameTitle}}ListResponse ` + "`" + `json:"{{.NamePlural}}" doc:"List of {{.NamePlural}}"` + "`" + `
		Count {{- " "}}int ` + "`" + `json:"count" doc:"Number of {{.NamePlural}}"` + "`" + `
	}
}

// List{{.NamePluralTitle}}Handler handles GET /{{.NamePlural}}
func (h *{{.NameTitle}}Handler) List{{.NamePluralTitle}}Handler(ctx context.Context, req *List{{.NamePluralTitle}}Request) (*List{{.NamePluralTitle}}Response, error) {
	items, err := h.{{.NameCamel}}Service.List{{.NamePluralTitle}}(nil)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch {{.NamePlural}}", err)
	}

	resp := &List{{.NamePluralTitle}}Response{}
	resp.Body.{{.NamePluralTitle}} = items
	resp.Body.Count = len(items)

	return resp, nil
}

// ============================================================================
// Search {{.NamePluralTitle}}
// ============================================================================

// Search{{.NamePluralTitle}}Request represents the search request
type Search{{.NamePluralTitle}}Request struct {
	Query string ` + "`" + `query:"q" doc:"Search query" example:"search term"` + "`" + `
}

// Search{{.NamePluralTitle}}Response represents the search response
type Search{{.NamePluralTitle}}Response struct {
	Body struct {
		{{.NamePluralTitle}} []*contracts.{{.NameTitle}}ListResponse ` + "`" + `json:"{{.NamePlural}}" doc:"Matching {{.NamePlural}}"` + "`" + `
		Count {{- " "}}int ` + "`" + `json:"count" doc:"Number of results"` + "`" + `
	}
}

// Search{{.NamePluralTitle}}Handler handles GET /{{.NamePlural}}/search?q=query
func (h *{{.NameTitle}}Handler) Search{{.NamePluralTitle}}Handler(ctx context.Context, req *Search{{.NamePluralTitle}}Request) (*Search{{.NamePluralTitle}}Response, error) {
	items, err := h.{{.NameCamel}}Service.Search{{.NamePluralTitle}}(req.Query)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to search {{.NamePlural}}", err)
	}

	resp := &Search{{.NamePluralTitle}}Response{}
	resp.Body.{{.NamePluralTitle}} = items
	resp.Body.Count = len(items)

	return resp, nil
}

// ============================================================================
// Get {{.NameTitle}}
// ============================================================================

// Get{{.NameTitle}}Request represents the request for getting a single {{.Name}}
type Get{{.NameTitle}}Request struct {
	{{.NameTitle}}ID string ` + "`" + `path:"{{.NameSnake}}_id" doc:"{{.NameTitle}} ID or slug"` + "`" + `
}

// Get{{.NameTitle}}Response represents the response for the get {{.Name}} endpoint
type Get{{.NameTitle}}Response struct {
	Body *contracts.{{.NameTitle}}DetailResponse
}

// Get{{.NameTitle}}Handler handles GET /{{.NamePlural}}/{{"{"}}{{.NameSnake}}_id}
func (h *{{.NameTitle}}Handler) Get{{.NameTitle}}Handler(ctx context.Context, req *Get{{.NameTitle}}Request) (*Get{{.NameTitle}}Response, error) {
	var result *contracts.{{.NameTitle}}DetailResponse
	var err error

	if id, parseErr := strconv.ParseUint(req.{{.NameTitle}}ID, 10, 32); parseErr == nil {
		result, err = h.{{.NameCamel}}Service.Get{{.NameTitle}}(uint(id))
	} else {
		result, err = h.{{.NameCamel}}Service.Get{{.NameTitle}}BySlug(req.{{.NameTitle}}ID)
	}

	if err != nil {
		return nil, huma.Error404NotFound("{{.NameTitle}} not found")
	}

	return &Get{{.NameTitle}}Response{Body: result}, nil
}

// ============================================================================
// Create {{.NameTitle}}
// ============================================================================

// Create{{.NameTitle}}Request represents the request for creating a {{.Name}}
type Create{{.NameTitle}}Request struct {
	Body struct {
{{- range .Fields}}
{{- if .IsRequired}}
		{{.GoName}} {{.GoType}} ` + "`" + `json:"{{.JSONTag}}" doc:"{{.GoName}}"` + "`" + `
{{- else if eq .Type "bool"}}
		{{.GoName}} {{.GoType}} ` + "`" + `json:"{{.JSONTag}}" required:"false" doc:"{{.GoName}}"` + "`" + `
{{- else}}
		{{.GoName}} {{goTypePtr .}} ` + "`" + `json:"{{.JSONTag}},omitempty" required:"false" doc:"{{.GoName}}"` + "`" + `
{{- end}}
{{- end}}
	}
}

// Create{{.NameTitle}}Response represents the response for creating a {{.Name}}
type Create{{.NameTitle}}Response struct {
	Body *contracts.{{.NameTitle}}DetailResponse
}

// Create{{.NameTitle}}Handler handles POST /{{.NamePlural}}
func (h *{{.NameTitle}}Handler) Create{{.NameTitle}}Handler(ctx context.Context, req *Create{{.NameTitle}}Request) (*Create{{.NameTitle}}Response, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

{{- if .NameField}}

	if req.Body.Name == "" {
		return nil, huma.Error400BadRequest("Name is required")
	}
{{- end}}

	serviceReq := &contracts.Create{{.NameTitle}}Request{
{{- range .Fields}}
		{{.GoName}}: req.Body.{{.GoName}},
{{- end}}
	}

	result, err := h.{{.NameCamel}}Service.Create{{.NameTitle}}(serviceReq)
	if err != nil {
		logger.FromContext(ctx).Error("create_{{.NameSnake}}_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create {{.Name}} (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "create_{{.NameSnake}}", "{{.NameSnake}}", result.ID, nil)
		}()
	}

	logger.FromContext(ctx).Info("{{.NameSnake}}_created",
		"{{.NameSnake}}_id", result.ID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &Create{{.NameTitle}}Response{Body: result}, nil
}

// ============================================================================
// Update {{.NameTitle}}
// ============================================================================

// Update{{.NameTitle}}Request represents the request for updating a {{.Name}}
type Update{{.NameTitle}}Request struct {
	{{.NameTitle}}ID string ` + "`" + `path:"{{.NameSnake}}_id" doc:"{{.NameTitle}} ID or slug"` + "`" + `
	Body struct {
{{- range .Fields}}
		{{.GoName}} {{goTypePtr .}} ` + "`" + `json:"{{.JSONTag}},omitempty" required:"false" doc:"{{.GoName}}"` + "`" + `
{{- end}}
	}
}

// Update{{.NameTitle}}Response represents the response for updating a {{.Name}}
type Update{{.NameTitle}}Response struct {
	Body *contracts.{{.NameTitle}}DetailResponse
}

// Update{{.NameTitle}}Handler handles PUT /{{.NamePlural}}/{{"{"}}{{.NameSnake}}_id}
func (h *{{.NameTitle}}Handler) Update{{.NameTitle}}Handler(ctx context.Context, req *Update{{.NameTitle}}Request) (*Update{{.NameTitle}}Response, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve ID
	{{.NameCamel}}ID, err := h.resolve{{.NameTitle}}ID(req.{{.NameTitle}}ID)
	if err != nil {
		return nil, err
	}

	serviceReq := &contracts.Update{{.NameTitle}}Request{
{{- range .Fields}}
		{{.GoName}}: req.Body.{{.GoName}},
{{- end}}
	}

	result, err := h.{{.NameCamel}}Service.Update{{.NameTitle}}({{.NameCamel}}ID, serviceReq)
	if err != nil {
		logger.FromContext(ctx).Error("update_{{.NameSnake}}_failed",
			"{{.NameSnake}}_id", {{.NameCamel}}ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update {{.Name}} (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "edit_{{.NameSnake}}", "{{.NameSnake}}", {{.NameCamel}}ID, nil)
		}()
	}

	logger.FromContext(ctx).Info("{{.NameSnake}}_updated",
		"{{.NameSnake}}_id", {{.NameCamel}}ID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &Update{{.NameTitle}}Response{Body: result}, nil
}

// ============================================================================
// Delete {{.NameTitle}}
// ============================================================================

// Delete{{.NameTitle}}Request represents the request for deleting a {{.Name}}
type Delete{{.NameTitle}}Request struct {
	{{.NameTitle}}ID string ` + "`" + `path:"{{.NameSnake}}_id" doc:"{{.NameTitle}} ID"` + "`" + `
}

// Delete{{.NameTitle}}Handler handles DELETE /{{.NamePlural}}/{{"{"}}{{.NameSnake}}_id}
func (h *{{.NameTitle}}Handler) Delete{{.NameTitle}}Handler(ctx context.Context, req *Delete{{.NameTitle}}Request) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve ID
	{{.NameCamel}}ID, err := h.resolve{{.NameTitle}}ID(req.{{.NameTitle}}ID)
	if err != nil {
		return nil, err
	}

	if err := h.{{.NameCamel}}Service.Delete{{.NameTitle}}({{.NameCamel}}ID); err != nil {
		logger.FromContext(ctx).Error("delete_{{.NameSnake}}_failed",
			"{{.NameSnake}}_id", {{.NameCamel}}ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to delete {{.Name}} (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "delete_{{.NameSnake}}", "{{.NameSnake}}", {{.NameCamel}}ID, nil)
		}()
	}

	logger.FromContext(ctx).Info("{{.NameSnake}}_deleted",
		"{{.NameSnake}}_id", {{.NameCamel}}ID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return nil, nil
}

// ============================================================================
// Helpers
// ============================================================================

// resolve{{.NameTitle}}ID tries to parse the ID as a number first, then falls back to slug lookup
func (h *{{.NameTitle}}Handler) resolve{{.NameTitle}}ID(idOrSlug string) (uint, error) {
	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		return uint(id), nil
	}

	// Fall back to slug lookup
	result, err := h.{{.NameCamel}}Service.Get{{.NameTitle}}BySlug(idOrSlug)
	if err != nil {
		return 0, huma.Error404NotFound("{{.NameTitle}} not found")
	}

	return result.ID, nil
}
`

var tmplFeatureTypes = `/**
 * {{.NameTitle}}-related TypeScript types
 *
 * These types match the backend API response structures
 * for {{.Name}} endpoints.
 */

export interface {{.NameTitle}}Detail {
  id: number
  slug: string
{{- range .Fields}}
  {{.JSONTag}}: {{tsTypeNull .}}
{{- end}}
  created_at: string
  updated_at: string
}

export interface {{.NameTitle}}ListItem {
  id: number
  slug: string
{{- range .Fields}}
  {{.JSONTag}}: {{tsTypeNull .}}
{{- end}}
}

export interface {{.NamePluralTitle}}ListResponse {
  {{.NamePlural}}: {{.NameTitle}}ListItem[]
  count: number
}
`

var tmplFeatureAPI = `/**
 * {{.NamePluralTitle}} API Configuration
 *
 * Co-located endpoint definitions and query keys for the {{.NamePlural}} feature.
 */

import { API_BASE_URL } from '@/lib/api-base'

// ============================================================================
// Endpoints
// ============================================================================

export const {{.NameCamel}}Endpoints = {
  LIST: ` + "`${API_BASE_URL}/{{.NamePlural}}`" + `,
  GET: (idOrSlug: string | number) => ` + "`${API_BASE_URL}/{{.NamePlural}}/${idOrSlug}`" + `,
  SEARCH: (q: string) => ` + "`${API_BASE_URL}/{{.NamePlural}}/search?q=${encodeURIComponent(q)}`" + `,
  CREATE: ` + "`${API_BASE_URL}/{{.NamePlural}}`" + `,
  UPDATE: (id: string | number) => ` + "`${API_BASE_URL}/{{.NamePlural}}/${id}`" + `,
  DELETE: (id: string | number) => ` + "`${API_BASE_URL}/{{.NamePlural}}/${id}`" + `,
} as const

// ============================================================================
// Query Keys
// ============================================================================

export const {{.NameCamel}}QueryKeys = {
  all: ['{{.NamePlural}}'] as const,
  list: (filters?: Record<string, unknown>) =>
    ['{{.NamePlural}}', 'list', filters] as const,
  detail: (idOrSlug: string | number) =>
    ['{{.NamePlural}}', 'detail', String(idOrSlug)] as const,
} as const
`

var tmplFeatureHooks = `'use client'

/**
 * {{.NamePluralTitle}} Hooks
 *
 * TanStack Query hooks for fetching {{.Name}} data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { createDetailHook } from '@/lib/hooks/factories'
import { {{.NameCamel}}Endpoints, {{.NameCamel}}QueryKeys } from '@/features/{{.NamePlural}}/api'
import type {
  {{.NamePluralTitle}}ListResponse,
  {{.NameTitle}}Detail,
} from '../types'

/**
 * Hook to fetch list of {{.NamePlural}}
 */
export function use{{.NamePluralTitle}}() {
  return useQuery({
    queryKey: {{.NameCamel}}QueryKeys.list(),
    queryFn: async (): Promise<{{.NamePluralTitle}}ListResponse> => {
      return apiRequest<{{.NamePluralTitle}}ListResponse>({{.NameCamel}}Endpoints.LIST, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000,
    placeholderData: keepPreviousData,
  })
}

/**
 * Hook to fetch a single {{.Name}} by ID or slug
 */
export const use{{.NameTitle}} = createDetailHook<{{.NameTitle}}Detail>(
  {{.NameCamel}}Endpoints.GET,
  {{.NameCamel}}QueryKeys.detail,
)
`

var tmplFeatureAdminHooks = `'use client'

/**
 * Admin {{.NameTitle}} Hooks
 *
 * TanStack Query mutations for admin {{.Name}} CRUD operations:
 * create, update, and delete.
 */

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { createInvalidateQueries } from '@/lib/queryClient'
import { {{.NameCamel}}Endpoints } from '@/features/{{.NamePlural}}/api'
import type { {{.NameTitle}}Detail } from '../types'

// ============================================================================
// Request Types
// ============================================================================

export interface Create{{.NameTitle}}Input {
{{- range .Fields}}
{{- if .IsRequired}}
  {{.JSONTag}}: {{.TSType}}
{{- else}}
  {{.JSONTag}}?: {{.TSType}} | null
{{- end}}
{{- end}}
}

export interface Update{{.NameTitle}}Input {
{{- range .Fields}}
  {{.JSONTag}}?: {{.TSType}} | null
{{- end}}
}

// ============================================================================
// Mutations
// ============================================================================

/**
 * Hook for creating a new {{.Name}} (admin only)
 */
export function useCreate{{.NameTitle}}() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (input: Create{{.NameTitle}}Input): Promise<{{.NameTitle}}Detail> => {
      return apiRequest<{{.NameTitle}}Detail>({{.NameCamel}}Endpoints.CREATE, {
        method: 'POST',
        body: JSON.stringify(input),
      })
    },
    onSuccess: () => {
      invalidateQueries.{{.NamePlural}}()
    },
  })
}

/**
 * Hook for updating an existing {{.Name}} (admin only)
 */
export function useUpdate{{.NameTitle}}() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      {{.NameCamel}}Id,
      data,
    }: {
      {{.NameCamel}}Id: number
      data: Update{{.NameTitle}}Input
    }): Promise<{{.NameTitle}}Detail> => {
      return apiRequest<{{.NameTitle}}Detail>(
        {{.NameCamel}}Endpoints.UPDATE({{.NameCamel}}Id),
        {
          method: 'PUT',
          body: JSON.stringify(data),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.{{.NamePlural}}()
    },
  })
}

/**
 * Hook for deleting a {{.Name}} (admin only)
 */
export function useDelete{{.NameTitle}}() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({{.NameCamel}}Id: number): Promise<void> => {
      return apiRequest<void>({{.NameCamel}}Endpoints.DELETE({{.NameCamel}}Id), {
        method: 'DELETE',
      })
    },
    onSuccess: () => {
      invalidateQueries.{{.NamePlural}}()
    },
  })
}
`

var tmplFeatureHooksIndex = `export {
  use{{.NamePluralTitle}},
  use{{.NameTitle}},
} from './use{{.NamePluralTitle}}'

export {
  type Create{{.NameTitle}}Input,
  type Update{{.NameTitle}}Input,
  useCreate{{.NameTitle}},
  useUpdate{{.NameTitle}},
  useDelete{{.NameTitle}},
} from './useAdmin{{.NamePluralTitle}}'
`

var tmplFeatureIndex = `// Public API for the {{.NamePlural}} feature module

// API (endpoints + query keys)
export { {{.NameCamel}}Endpoints, {{.NameCamel}}QueryKeys } from './api'

// Types
export type {
  {{.NameTitle}}Detail,
  {{.NameTitle}}ListItem,
  {{.NamePluralTitle}}ListResponse,
} from './types'

// Hooks
export {
  use{{.NamePluralTitle}},
  use{{.NameTitle}},
} from './hooks'

export {
  type Create{{.NameTitle}}Input,
  type Update{{.NameTitle}}Input,
  useCreate{{.NameTitle}},
  useUpdate{{.NameTitle}},
  useDelete{{.NameTitle}},
} from './hooks'
`
