package catalog

import (
	"os"
	"regexp"
	"slices"
	"testing"
)

// frontendTagTypesPath is the frontend module that states the same category
// rules this file states. Relative to this package's directory.
const frontendTagTypesPath = "../../../../frontend/features/tags/types.ts"

var (
	// Anchored on the declaration, so neither pattern can match inside a longer
	// identifier such as DESCRIPTIVE_TAG_CATEGORIES, whose own initializer would
	// otherwise let this test compare a derived list against itself.
	frontendCategoryListPattern = regexp.MustCompile(`(?sm)^export const TAG_CATEGORIES\s*=\s*\[(.*?)\]`)
	frontendCrewPattern         = regexp.MustCompile(`(?m)^export const TAG_CATEGORY_CREW\s*=\s*'([^']+)'`)
	frontendQuotedPattern       = regexp.MustCompile(`'([^']+)'`)
)

// TestDescriptiveTagCategoriesMatchFrontend pins the two builds of the same
// rule to each other.
//
// Both sides answer "may this tag be ranked against the others", and a surface
// that disagrees with the server ranks a booker as a genre or drops a genre as
// a booker. Neither the compiler nor the OpenAPI contract can see the pair, so
// this test is the only thing that does.
//
// It pins the INPUTS the two predicates are built from: the category vocabulary
// and the one category that is not descriptive. It cannot see the TypeScript's
// control flow, so a frontend edit that keeps both constants and changes what
// the function does with them passes here and is caught only by review.
func TestDescriptiveTagCategoriesMatchFrontend(t *testing.T) {
	source, err := os.ReadFile(frontendTagTypesPath)
	if err != nil {
		t.Fatalf("cannot read %s: %v\n"+
			"This test pins the backend tag-category rules to the frontend's. "+
			"If that module moved, repoint this constant; do not delete the pin.",
			frontendTagTypesPath, err)
	}

	listMatch := frontendCategoryListPattern.FindSubmatch(source)
	if listMatch == nil {
		t.Fatalf("no TAG_CATEGORIES array found in %s", frontendTagTypesPath)
	}
	var frontendCategories []string
	for _, quoted := range frontendQuotedPattern.FindAllSubmatch(listMatch[1], -1) {
		frontendCategories = append(frontendCategories, string(quoted[1]))
	}

	crewMatch := frontendCrewPattern.FindSubmatch(source)
	if crewMatch == nil {
		t.Fatalf("no TAG_CATEGORY_CREW constant found in %s", frontendTagTypesPath)
	}
	frontendCrew := string(crewMatch[1])

	if got, want := slices.Sorted(slices.Values(frontendCategories)), slices.Sorted(slices.Values(TagCategories)); !slices.Equal(got, want) {
		t.Errorf("tag category vocabulary differs: frontend %v, backend %v", got, want)
	}

	if frontendCrew != TagCategoryCrew {
		t.Errorf("the non-descriptive category differs: frontend %q, backend %q", frontendCrew, TagCategoryCrew)
	}

	// The derivation both sides perform, run over each side's own inputs.
	var frontendDescriptive []string
	for _, category := range frontendCategories {
		if category != frontendCrew {
			frontendDescriptive = append(frontendDescriptive, category)
		}
	}
	var backendDescriptive []string
	for _, category := range TagCategories {
		if IsDescriptiveTagCategory(category) {
			backendDescriptive = append(backendDescriptive, category)
		}
	}
	if got, want := slices.Sorted(slices.Values(frontendDescriptive)), slices.Sorted(slices.Values(backendDescriptive)); !slices.Equal(got, want) {
		t.Errorf("descriptive categories differ: frontend %v, backend %v", got, want)
	}
	if len(backendDescriptive) == len(TagCategories) {
		t.Error("every category is descriptive, so the two lists agree vacuously")
	}
}
