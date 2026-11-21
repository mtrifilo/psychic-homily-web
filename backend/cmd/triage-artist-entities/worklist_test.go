package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWorklist(t *testing.T) {
	t.Run("reads the reviewed columns, trims, lowercases the action and ignores extra columns", func(t *testing.T) {
		rows, err := parseWorklist(strings.NewReader(
			"\xef\xbb\xbfartist_slug,action,target,show_slug,note,heuristics\n" +
				" live-at-launchpad , REMOVE , title , 2026-11-21-x-at-launchpad ,subtitle line,H1\n" +
				",,,,,\n" +
				"lone-stranger-friends,rename,Lone Stranger,,,H1\n"))
		require.NoError(t, err)
		assert.Equal(t, []worklistRow{
			{Line: 2, ArtistSlug: "live-at-launchpad", Action: "remove", Target: "title", ShowSlug: "2026-11-21-x-at-launchpad", Note: "subtitle line"},
			{Line: 4, ArtistSlug: "lone-stranger-friends", Action: "rename", Target: "Lone Stranger"},
		}, rows)
	})

	t.Run("show_slug and note are optional", func(t *testing.T) {
		rows, err := parseWorklist(strings.NewReader("action,target,artist_slug\nkeep,,ganja-white-night\n"))
		require.NoError(t, err)
		assert.Equal(t, []worklistRow{{Line: 2, ArtistSlug: "ganja-white-night", Action: "keep"}}, rows)
	})

	for name, tc := range map[string]struct{ in, wantErr string }{
		"a file that still carries a suggestion column": {
			in:      "artist_slug,action,target,suggested_action,suggested_target\na,keep,,remove,\n",
			wantErr: `still carries the suggestion column "suggested_action"`,
		},
		"an unreviewed suggestions file": {
			in:      "artist_slug,suggested_action,suggested_target\na,remove,\n",
			wantErr: "suggestion column",
		},
		"a missing target column": {in: "artist_slug,action\na,keep\n", wantErr: `no "target" column`},
		"a misspelt optional column": {
			in:      "artist_slug,action,target,show_slugs\na,remove,,a-show\n",
			wantErr: `unknown column "show_slugs"`,
		},
		"a repeated column":    {in: "artist_slug,action,target,action\na,keep,,merge\n", wantErr: "twice"},
		"an empty file":        {in: "", wantErr: "empty"},
		"a header and no rows": {in: "artist_slug,action,target\n", wantErr: "no rows"},
		"a ragged row":         {in: "artist_slug,action,target\na,keep\n", wantErr: "read worklist"},
	} {
		t.Run("refuses "+name, func(t *testing.T) {
			_, err := parseWorklist(strings.NewReader(tc.in))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestSplitActNames(t *testing.T) {
	acts, err := splitActNames(" Darby O'Trill | Mikey Rotten ", "Darby O'Trill and Mikey Rotten")
	require.NoError(t, err)
	assert.Equal(t, []string{"Darby O'Trill", "Mikey Rotten"}, acts)

	acts, err = splitActNames("Tara Zorthian|Ranch House Motel|Stine", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"Tara Zorthian", "Ranch House Motel", "Stine"}, acts)

	for target, wantErr := range map[string]string{
		"Darby O'Trill":                 "two or more act names",
		"Darby O'Trill | ":              "empty act name",
		"Darby O'Trill | darby o'trill": "twice",
		"darby-otrill | Mikey Rotten":   "looks like a slug",
		"Darby and Mikey | Mikey":       "joined entity's own name",
	} {
		_, err := splitActNames(target, "Darby and Mikey")
		require.Error(t, err, target)
		assert.Contains(t, err.Error(), wantErr, target)
	}
}

// Rows that cannot describe a decision are refused before the database is read.
func TestMalformedRowsAreRefused(t *testing.T) {
	rows := []worklistRow{
		{Line: 2, Action: "keep"},
		{Line: 3, ArtistSlug: "a", Action: "rename"},
		{Line: 4, ArtistSlug: "b", Action: "rename", Target: "!!!"},
		{Line: 5, ArtistSlug: "c", Action: "rename", Target: "some-slug-shape"},
		{Line: 6, ArtistSlug: "d", Action: "merge"},
		{Line: 7, ArtistSlug: "e", Action: "merge", Target: "e"},
		{Line: 8, ArtistSlug: "f", Action: "remove", Target: "headline"},
		{Line: 9, ArtistSlug: "g", Action: "split"},
		{Line: 10, ArtistSlug: "h", Action: "split", Target: "Only One"},
		{Line: 11, ArtistSlug: "i", Action: "keep", Target: "x"},
		{Line: 12, ArtistSlug: "j", Action: "demolish"},
		{Line: 13, ArtistSlug: "k", Action: "rename", Target: "K", ShowSlug: "a-show"},
	}
	plans := make([]*rowPlan, len(rows))
	for i, r := range rows {
		plans[i] = &rowPlan{Row: r}
	}

	refuseMalformedRows(plans)

	want := []string{
		"artist_slug is empty",
		"rename needs a target",
		"has no letters or digits",
		"looks like a slug",
		"merge needs a target",
		"is the artist itself",
		`remove takes no target, or "title"`,
		"split needs a target",
		"two or more act names",
		"keep takes no target",
		`unknown action "demolish"`,
		"show_slug applies to remove and split only",
	}
	for i, p := range plans {
		assert.Equal(t, statusRefused, p.Status, "line %d", p.Row.Line)
		assert.Contains(t, strings.Join(p.Reasons, "; "), want[i], "line %d", p.Row.Line)
	}
}

// The named env file decides the database even when the shell already exports
// one.
func TestLoadNamedEnvOverridesAnExportedDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://exported.example/prod")
	file := filepath.Join(t.TempDir(), "stage.env")
	require.NoError(t, os.WriteFile(file, []byte("DATABASE_URL=postgres://named.example/stage\n"), 0o600))

	require.NoError(t, loadNamedEnv(file))

	assert.Equal(t, "postgres://named.example/stage", os.Getenv("DATABASE_URL"))
}

// A named env file that does not set DATABASE_URL is refused, and the process
// environment is left as it was.
func TestLoadNamedEnvRefusesAFileWithoutADatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://exported.example/prod")
	t.Setenv("ENVIRONMENT", "development")
	file := filepath.Join(t.TempDir(), "partial.env")
	require.NoError(t, os.WriteFile(file, []byte("ENVIRONMENT=stage\n"), 0o600))

	err := loadNamedEnv(file)

	require.ErrorContains(t, err, "sets no DATABASE_URL")
	assert.Equal(t, "postgres://exported.example/prod", os.Getenv("DATABASE_URL"))
	assert.Equal(t, "development", os.Getenv("ENVIRONMENT"), "nothing from the refused file is loaded")
}

func TestValidateFlags(t *testing.T) {
	require.NoError(t, validateFlags(false, false, "stage.env", "", 2))
	require.NoError(t, validateFlags(false, true, "stage.env", "", 2))
	require.NoError(t, validateFlags(true, false, "prod.env", "applied.jsonl", 2))
	for name, tc := range map[string]struct {
		confirm, dryRun bool
		env, record     string
		actor           uint
		want            string
	}{
		"both modes":             {confirm: true, dryRun: true, env: "e", record: "r", actor: 2, want: "cannot be combined"},
		"no env":                 {env: "", actor: 2, want: "--env is required"},
		"no actor":               {env: "e", want: "--actor is required"},
		"confirm with no record": {confirm: true, env: "e", actor: 2, want: "--confirm requires --record"},
		"record on a dry run":    {env: "e", record: "r", actor: 2, want: "--record is written only by --confirm"},
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, validateFlags(tc.confirm, tc.dryRun, tc.env, tc.record, tc.actor), tc.want)
		})
	}
}

// The record file is opened for append and never truncated, and a path that
// cannot be opened is refused before anything runs.
func TestOpenRecordAppendsAndRefusesAnUnopenablePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "applied.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("{\"earlier\":true}\n"), 0o600))

	f, err := openRecord(path)
	require.NoError(t, err)
	_, err = f.WriteString("{\"later\":true}\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "{\"earlier\":true}\n{\"later\":true}\n", string(got))

	_, err = openRecord(filepath.Join(t.TempDir(), "no-such-dir", "applied.jsonl"))
	require.ErrorContains(t, err, "cannot be opened for append")
}

// A name that prints like another name but does not match it is refused.
func TestNameProblem(t *testing.T) {
	assert.Empty(t, nameProblem("Darby O'Trill"))
	assert.Empty(t, nameProblem("Beyoncé"))
	for name, want := range map[string]string{
		"Foo\u00a0Bar": "space character U+00A0",
		"Foo\u200bBar": "invisible character U+200B",
		"Foo\nBar":     "invisible character U+000A",
		"Foo\u2028Bar": "invisible character U+2028",
		"Foo  Bar":     "two spaces in a row",
	} {
		assert.Contains(t, nameProblem(name), want, "%q", name)
	}
	_, err := splitActNames("Darby | Mikey\u00a0Rotten", "")
	assert.ErrorContains(t, err, "space character U+00A0")
}

// Names from the database cannot rewrite the line the operator reads.
func TestCleanReplacesControlAndFormatCharacters(t *testing.T) {
	assert.Equal(t, "LIVE?AT?LAUNCHPAD?", clean("LIVE\nAT\u202eLAUNCHPAD\u2028"))
	assert.Equal(t, "Beyoncé", clean("Beyoncé"))
}

func TestEnvIsRequiredOnEveryRun(t *testing.T) {
	for _, envFile := range []string{"", "   "} {
		require.ErrorContains(t, requireEnvFlag(envFile), "--env is required on every run, dry run included")
	}
	require.NoError(t, requireEnvFlag("prod.env"))
	require.ErrorContains(t, loadNamedEnv(filepath.Join(t.TempDir(), "absent.env")), "read env file")
}

// The printed target names the host and database and never the credentials.
func TestRedactDBHostDropsTheCredentials(t *testing.T) {
	got := redactDBHost("postgres://admin:s3cret-pass@db.example.internal:5432/psychic?sslmode=require")
	assert.Equal(t, "db.example.internal:5432/psychic", got)
	assert.NotContains(t, got, "s3cret")
	assert.NotContains(t, got, "admin")
	assert.Equal(t, "(unparseable)", redactDBHost("not a url"))
}
