package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertNilDBError calls fn and asserts it returns a "database not initialized" error.
// Use this for service methods that return only an error.
func AssertNilDBError(t *testing.T, fn func() error) {
	t.Helper()
	err := fn()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database not initialized")
}

// AssertNilDBErrorWithResult calls fn and asserts it returns a "database not initialized" error.
// The result value is ignored (it should be zero-value).
// Use this for service methods that return (SomeType, error).
func AssertNilDBErrorWithResult[T any](t *testing.T, fn func() (T, error)) {
	t.Helper()
	_, err := fn()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database not initialized")
}
