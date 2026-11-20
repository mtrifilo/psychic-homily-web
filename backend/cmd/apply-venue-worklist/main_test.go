package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestLoadNamedEnvRefusesAMissingFile(t *testing.T) {
	require.ErrorContains(t, loadNamedEnv(filepath.Join(t.TempDir(), "absent.env")), "read env file")
}
