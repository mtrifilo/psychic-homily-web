package config

import (
	"os"
	"testing"
)

func TestGetEnvAsInt(t *testing.T) {
	tests := []struct {
		name         string
		envKey       string
		envValue     string
		defaultValue int
		expected     int
		shouldLog    bool
	}{
		{
			name:         "valid positive integer",
			envKey:       "TEST_PORT",
			envValue:     "8080",
			defaultValue: 3000,
			expected:     8080,
			shouldLog:    false,
		},
		{
			name:         "valid negative integer",
			envKey:       "TEST_TIMEOUT",
			envValue:     "-30",
			defaultValue: 60,
			expected:     -30,
			shouldLog:    false,
		},
		{
			name:         "valid zero",
			envKey:       "TEST_RETRIES",
			envValue:     "0",
			defaultValue: 3,
			expected:     0,
			shouldLog:    false,
		},
		{
			name:         "invalid integer string",
			envKey:       "TEST_PORT",
			envValue:     "not-a-number",
			defaultValue: 3000,
			expected:     3000,
			shouldLog:    true,
		},
		{
			name:         "empty string",
			envKey:       "TEST_PORT",
			envValue:     "",
			defaultValue: 3000,
			expected:     3000,
			shouldLog:    true,
		},
		{
			name:         "decimal number",
			envKey:       "TEST_PORT",
			envValue:     "8080.5",
			defaultValue: 3000,
			expected:     3000,
			shouldLog:    true,
		},
		{
			name:         "large number",
			envKey:       "TEST_LARGE",
			envValue:     "2147483647",
			defaultValue: 1000,
			expected:     2147483647,
			shouldLog:    false,
		},
		{
			name:         "very large number",
			envKey:       "TEST_VERY_LARGE",
			envValue:     "9223372036854775807",
			defaultValue: 1000,
			expected:     9223372036854775807,
			shouldLog:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variable
			if tt.envValue != "" {
				os.Setenv(tt.envKey, tt.envValue)
				defer os.Unsetenv(tt.envKey)
			} else {
				// Ensure the environment variable is not set
				os.Unsetenv(tt.envKey)
			}

			// Call the function
			result := getEnvAsInt(tt.envKey, tt.defaultValue)

			// Check the result
			if result != tt.expected {
				t.Errorf("getEnvAsInt(%q, %d) = %d, want %d", tt.envKey, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

func TestGetEnvAsIntEnvironmentIsolation(t *testing.T) {
	// Test that environment variables don't interfere with each other
	os.Setenv("TEST_A", "100")
	os.Setenv("TEST_B", "200")
	defer func() {
		os.Unsetenv("TEST_A")
		os.Unsetenv("TEST_B")
	}()

	// Test that each variable returns its own value
	if result := getEnvAsInt("TEST_A", 0); result != 100 {
		t.Errorf("getEnvAsInt(\"TEST_A\", 0) = %d, want 100", result)
	}

	if result := getEnvAsInt("TEST_B", 0); result != 200 {
		t.Errorf("getEnvAsInt(\"TEST_B\", 0) = %d, want 200", result)
	}

	// Test that unset variable returns default
	if result := getEnvAsInt("TEST_C", 300); result != 300 {
		t.Errorf("getEnvAsInt(\"TEST_C\", 300) = %d, want 300", result)
	}
}

func TestGetEnvAsIntEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		envKey       string
		envValue     string
		defaultValue int
		expected     int
	}{
		{
			name:         "whitespace only",
			envKey:       "TEST_WHITESPACE",
			envValue:     "   ",
			defaultValue: 100,
			expected:     100,
		},
		{
			name:         "whitespace around valid number",
			envKey:       "TEST_WHITESPACE_NUMBER",
			envValue:     "  123  ",
			defaultValue: 100,
			expected:     100, // strconv.Atoi doesn't trim whitespace
		},
		{
			name:         "hexadecimal string",
			envKey:       "TEST_HEX",
			envValue:     "0xFF",
			defaultValue: 100,
			expected:     100,
		},
		{
			name:         "binary string",
			envKey:       "TEST_BINARY",
			envValue:     "1010",
			defaultValue: 100,
			expected:     1010, // This is valid as decimal
		},
		{
			name:         "scientific notation",
			envKey:       "TEST_SCIENTIFIC",
			envValue:     "1e3",
			defaultValue: 100,
			expected:     100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(tt.envKey, tt.envValue)
			defer os.Unsetenv(tt.envKey)

			result := getEnvAsInt(tt.envKey, tt.defaultValue)

			if result != tt.expected {
				t.Errorf("getEnvAsInt(%q, %d) = %d, want %d", tt.envKey, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

// Benchmark test for performance
func BenchmarkGetEnvAsInt(b *testing.B) {
	os.Setenv("BENCHMARK_TEST", "12345")
	defer os.Unsetenv("BENCHMARK_TEST")

	for i := 0; i < b.N; i++ {
		getEnvAsInt("BENCHMARK_TEST", 0)
	}
}

func BenchmarkGetEnvAsIntDefault(b *testing.B) {
	os.Unsetenv("BENCHMARK_TEST_DEFAULT")

	for i := 0; i < b.N; i++ {
		getEnvAsInt("BENCHMARK_TEST_DEFAULT", 12345)
	}
} 
