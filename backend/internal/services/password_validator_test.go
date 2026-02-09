package services

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundTripFunc adapts a function to http.RoundTripper for mocking HTTP calls.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// newMockValidator creates a PasswordValidator with a mock HTTP transport.
func newMockValidator(transport http.RoundTripper) *PasswordValidator {
	v := NewPasswordValidator()
	v.httpClient = &http.Client{Transport: transport}
	return v
}

// mockHIBPResponse returns a RoundTripper that serves the given body for HIBP range requests.
func mockHIBPResponse(statusCode int, body string) roundTripFunc {
	return func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: statusCode,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}
}

// mockHIBPError returns a RoundTripper that always returns a network error.
func mockHIBPError(err error) roundTripFunc {
	return func(req *http.Request) (*http.Response, error) {
		return nil, err
	}
}

// hibpSuffix computes the HIBP SHA-1 suffix (chars 5+) for a password.
func hibpSuffix(password string) string {
	hash := sha1.Sum([]byte(password))
	return strings.ToUpper(hex.EncodeToString(hash[:]))[5:]
}

// --- NewPasswordValidator ---

func TestNewPasswordValidator(t *testing.T) {
	v := NewPasswordValidator()
	assert.NotNil(t, v)
	assert.NotNil(t, v.httpClient)
	assert.NotEmpty(t, v.commonPasswords)
	// Spot-check a few known common passwords
	assert.True(t, v.commonPasswords["password"])
	assert.True(t, v.commonPasswords["123456"])
	assert.True(t, v.commonPasswords["qwerty"])
}

// --- IsCommonPassword ---

func TestIsCommonPassword_Known(t *testing.T) {
	v := NewPasswordValidator()
	commonOnes := []string{"password", "123456", "qwerty", "admin", "letmein", "welcome"}
	for _, pw := range commonOnes {
		assert.True(t, v.IsCommonPassword(pw), "expected %q to be common", pw)
	}
}

func TestIsCommonPassword_CaseInsensitive(t *testing.T) {
	v := NewPasswordValidator()
	assert.True(t, v.IsCommonPassword("PASSWORD"))
	assert.True(t, v.IsCommonPassword("Password"))
	assert.True(t, v.IsCommonPassword("QWERTY"))
}

func TestIsCommonPassword_NotCommon(t *testing.T) {
	v := NewPasswordValidator()
	assert.False(t, v.IsCommonPassword("xK9$mPq2vL!nR8wJ"))
	assert.False(t, v.IsCommonPassword("correcthorsebatterystaple"))
}

// --- IsBreached ---

func TestIsBreached_Found(t *testing.T) {
	// "password" SHA-1 = 5BAA61E4C9B93F3F0682250B6CF8331B7EE68FD8
	// suffix = 1E4C9B93F3F0682250B6CF8331B7EE68FD8
	suffix := hibpSuffix("password")
	body := "1D2DA4053E34E76F6576ED1DA63134B5E2A:3\r\n" + suffix + ":10000\r\nABCDEF1234567890ABCDEF1234567890ABC:5\r\n"
	v := newMockValidator(mockHIBPResponse(http.StatusOK, body))

	breached, err := v.IsBreached("password")
	require.NoError(t, err)
	assert.True(t, breached)
}

func TestIsBreached_NotFound(t *testing.T) {
	body := "1D2DA4053E34E76F6576ED1DA63134B5E2A:3\r\nABCDEF1234567890ABCDEF1234567890ABC:5\r\n"
	v := newMockValidator(mockHIBPResponse(http.StatusOK, body))

	breached, err := v.IsBreached("password")
	require.NoError(t, err)
	assert.False(t, breached)
}

func TestIsBreached_CaseInsensitiveSuffixMatch(t *testing.T) {
	// Return lowercase suffix — should still match via EqualFold
	suffix := strings.ToLower(hibpSuffix("password"))
	body := suffix + ":10000\r\n"
	v := newMockValidator(mockHIBPResponse(http.StatusOK, body))

	breached, err := v.IsBreached("password")
	require.NoError(t, err)
	assert.True(t, breached)
}

func TestIsBreached_APIError_NonOK(t *testing.T) {
	v := newMockValidator(mockHIBPResponse(http.StatusServiceUnavailable, ""))

	breached, err := v.IsBreached("password")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 503")
	assert.False(t, breached)
}

func TestIsBreached_NetworkError(t *testing.T) {
	v := newMockValidator(mockHIBPError(fmt.Errorf("connection refused")))

	breached, err := v.IsBreached("password")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query pwned passwords")
	assert.False(t, breached)
}

func TestIsBreached_EmptyResponse(t *testing.T) {
	v := newMockValidator(mockHIBPResponse(http.StatusOK, ""))

	breached, err := v.IsBreached("password")
	require.NoError(t, err)
	assert.False(t, breached)
}

func TestIsBreached_RequestHeaders(t *testing.T) {
	var capturedReq *http.Request
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})
	v := newMockValidator(transport)
	_, _ = v.IsBreached("test")

	require.NotNil(t, capturedReq)
	assert.Equal(t, "PsychicHomily-PasswordCheck", capturedReq.Header.Get("User-Agent"))
	assert.Equal(t, "true", capturedReq.Header.Get("Add-Padding"))
	assert.Contains(t, capturedReq.URL.Path, "/range/")
}

func TestIsBreached_UsesFirst5CharsAsPrefix(t *testing.T) {
	var capturedURL string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})
	v := newMockValidator(transport)

	// "password" SHA-1 prefix = 5BAA6
	_, _ = v.IsBreached("password")
	assert.Contains(t, capturedURL, "/range/5BAA6")
}

// --- ValidatePassword ---

func TestValidatePassword_ValidPassword(t *testing.T) {
	// A long, unique password that's not common and not breached
	body := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA0:1\r\n"
	v := newMockValidator(mockHIBPResponse(http.StatusOK, body))

	result, err := v.ValidatePassword("xK9$mPq2vL!nR8wJ")
	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
	assert.Empty(t, result.Warnings)
}

func TestValidatePassword_TooShort(t *testing.T) {
	v := newMockValidator(mockHIBPResponse(http.StatusOK, ""))

	result, err := v.ValidatePassword("short")
	require.NoError(t, err)
	assert.False(t, result.Valid)

	var hasLengthErr bool
	for _, e := range result.Errors {
		if strings.Contains(e, fmt.Sprintf("at least %d characters", MinPasswordLength)) {
			hasLengthErr = true
		}
	}
	assert.True(t, hasLengthErr, "expected minimum length error")
}

func TestValidatePassword_TooLong(t *testing.T) {
	v := newMockValidator(mockHIBPResponse(http.StatusOK, ""))

	longPw := strings.Repeat("a", MaxPasswordLength+1)
	result, err := v.ValidatePassword(longPw)
	require.NoError(t, err)
	assert.False(t, result.Valid)

	var hasMaxErr bool
	for _, e := range result.Errors {
		if strings.Contains(e, fmt.Sprintf("no more than %d characters", MaxPasswordLength)) {
			hasMaxErr = true
		}
	}
	assert.True(t, hasMaxErr, "expected maximum length error")
}

func TestValidatePassword_CommonPassword(t *testing.T) {
	v := newMockValidator(mockHIBPResponse(http.StatusOK, ""))

	result, err := v.ValidatePassword("password")
	require.NoError(t, err)
	assert.False(t, result.Valid)

	var hasCommonErr bool
	for _, e := range result.Errors {
		if strings.Contains(e, "too common") {
			hasCommonErr = true
		}
	}
	assert.True(t, hasCommonErr, "expected 'too common' error")
}

func TestValidatePassword_BreachedPassword(t *testing.T) {
	// Use a 12+ char, non-common password and mock it as breached
	pw := "xK9mPq2vLnR8"
	suffix := hibpSuffix(pw)
	body := suffix + ":5000\r\n"
	v := newMockValidator(mockHIBPResponse(http.StatusOK, body))

	result, err := v.ValidatePassword(pw)
	require.NoError(t, err)
	assert.False(t, result.Valid)

	var hasBreachErr bool
	for _, e := range result.Errors {
		if strings.Contains(e, "data breach") {
			hasBreachErr = true
		}
	}
	assert.True(t, hasBreachErr, "expected 'data breach' error")
}

func TestValidatePassword_APIErrorAddsWarning(t *testing.T) {
	v := newMockValidator(mockHIBPError(fmt.Errorf("connection refused")))

	result, err := v.ValidatePassword("xK9$mPq2vL!nR8wJ")
	require.NoError(t, err)
	assert.True(t, result.Valid, "should still be valid when breach check fails")
	assert.NotEmpty(t, result.Warnings)
	assert.Contains(t, result.Warnings[0], "breach database")
}

func TestValidatePassword_MultipleErrors(t *testing.T) {
	// "admin" is too short AND common
	v := newMockValidator(mockHIBPResponse(http.StatusOK, ""))

	result, err := v.ValidatePassword("admin")
	require.NoError(t, err)
	assert.False(t, result.Valid)
	assert.GreaterOrEqual(t, len(result.Errors), 2, "expected at least 2 errors (short + common)")
}

func TestValidatePassword_ExactMinLength(t *testing.T) {
	// Exactly MinPasswordLength chars, not common, not breached
	pw := strings.Repeat("x", MinPasswordLength)
	v := newMockValidator(mockHIBPResponse(http.StatusOK, ""))

	result, err := v.ValidatePassword(pw)
	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

func TestValidatePassword_ExactMaxLength(t *testing.T) {
	// Exactly MaxPasswordLength chars, not common, not breached
	pw := strings.Repeat("y", MaxPasswordLength)
	v := newMockValidator(mockHIBPResponse(http.StatusOK, ""))

	result, err := v.ValidatePassword(pw)
	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

// --- CalculatePasswordStrength ---

func TestCalculatePasswordStrength_EmptyPassword(t *testing.T) {
	score := CalculatePasswordStrength("")
	assert.Equal(t, 0, score)
}

func TestCalculatePasswordStrength_ShortLowercaseOnly(t *testing.T) {
	// 8 chars lowercase: no length bonus, just lowercase=10
	score := CalculatePasswordStrength("abcdefgh")
	assert.Equal(t, 10, score)
}

func TestCalculatePasswordStrength_12CharsLowercaseOnly(t *testing.T) {
	// 12 chars lowercase: length(20) + lower(10) + entropy if ratio > 0.5
	// "abcdefghijkl" has 12 unique/12 = 1.0 > 0.5 → +10
	score := CalculatePasswordStrength("abcdefghijkl")
	assert.Equal(t, 40, score)
}

func TestCalculatePasswordStrength_16CharsAllTypes(t *testing.T) {
	// "aB3!aB3!aB3!aB3!" — 16 chars, has lower+upper+digit+special
	// length: 20+10=30, variety: 4*10=40, entropy: 4 unique / 16 = 0.25 < 0.5 → 0
	score := CalculatePasswordStrength("aB3!aB3!aB3!aB3!")
	assert.Equal(t, 70, score)
}

func TestCalculatePasswordStrength_20CharsHighEntropy(t *testing.T) {
	// 20 unique chars, all types, high entropy ratio
	pw := "xK9$mPq2vL!nR8wJzF5@"
	score := CalculatePasswordStrength(pw)
	// length: 20+10+10=40, variety: 40, entropy: ratio=1.0 → 10+10=20 → total 100
	assert.Equal(t, 100, score)
}

func TestCalculatePasswordStrength_CappedAt100(t *testing.T) {
	pw := "xK9$mPq2vL!nR8wJzF5@abcXYZ"
	score := CalculatePasswordStrength(pw)
	assert.LessOrEqual(t, score, 100)
}

func TestCalculatePasswordStrength_DigitsOnly(t *testing.T) {
	// 12 digits: length(20) + digit(10) + entropy: 10/12=0.83 > 0.5 → +10
	score := CalculatePasswordStrength("123456789012")
	assert.Equal(t, 40, score)
}

func TestCalculatePasswordStrength_LowEntropy(t *testing.T) {
	// "aaaaaaaaaaaa" — 12 chars, 1 unique = ratio 0.08 → no entropy bonus
	score := CalculatePasswordStrength("aaaaaaaaaaaa")
	assert.Equal(t, 30, score) // 20(length) + 10(lower)
}

func TestCalculatePasswordStrength_16CharsHighEntropy(t *testing.T) {
	// 16 chars, all unique lowercase: length(30) + lower(10) + entropy: 16/16=1.0 → both bonuses (10+10)
	pw := "abcdefghijklmnop"
	score := CalculatePasswordStrength(pw)
	assert.Equal(t, 60, score) // 30 + 10 + 20
}

// --- GetStrengthLabel ---

func TestGetStrengthLabel_Boundaries(t *testing.T) {
	tests := []struct {
		score    int
		expected string
	}{
		{0, "Weak"},
		{10, "Weak"},
		{29, "Weak"},
		{30, "Fair"},
		{49, "Fair"},
		{50, "Good"},
		{69, "Good"},
		{70, "Strong"},
		{89, "Strong"},
		{90, "Excellent"},
		{100, "Excellent"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("score_%d", tc.score), func(t *testing.T) {
			assert.Equal(t, tc.expected, GetStrengthLabel(tc.score))
		})
	}
}
