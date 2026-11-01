package auth

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// Password validation constants
const (
	MinPasswordLength = 12
	MaxPasswordLength = 128
)

// PasswordValidator validates passwords against security requirements
type PasswordValidator struct {
	httpClient      *http.Client
	commonPasswords map[string]bool

	// unavailableLogMu guards unavailableLoggedAt, when this validator last
	// wrote password_breach_check_unavailable.
	unavailableLogMu    sync.Mutex
	unavailableLoggedAt time.Time

	// Test overrides. Each zero value selects the production behavior.
	now    func() time.Time // replaces time.Now for the unavailable log's throttle and elapsed time
	logger *slog.Logger     // replaces logger.Default() for the unavailable log
}

const (
	// breachCheckTimeout bounds every outbound Pwned Passwords range request.
	breachCheckTimeout = 5 * time.Second

	// breachCheckUnavailableLogInterval is the minimum spacing between two
	// password_breach_check_unavailable lines from one validator, so an outage
	// writes one line per interval instead of one per password checked.
	breachCheckUnavailableLogInterval = time.Minute
)

// breachCheckStatusError is a range response whose status is not 200.
type breachCheckStatusError struct {
	statusCode int
}

func (e *breachCheckStatusError) Error() string {
	return fmt.Sprintf("pwned passwords API returned status %d", e.statusCode)
}

// errBreachCheckBadBody is a 200 range response that holds no range record:
// the body is empty, no line has the record shape, or a line is too long for
// the scanner to read.
var errBreachCheckBadBody = errors.New("pwned passwords API returned a body with no range records")

// rangeRecordSuffixLength is the length of the SHA-1 hex suffix that opens a
// range record: 40 hex characters less the 5 sent as the prefix.
const rangeRecordSuffixLength = 35

// NewPasswordValidator creates a new password validator
func NewPasswordValidator() *PasswordValidator {
	return NewPasswordValidatorWithTransport(nil)
}

// NewPasswordValidatorWithTransport is NewPasswordValidator with every Pwned
// Passwords range request carried by transport, which is how a test answers the
// breach check in-process instead of over the network. A nil transport means
// http.DefaultTransport. The client around the transport is built here, so
// every request carries the breachCheckTimeout deadline whatever transport the
// caller passes; a transport that ignores its request's context is not bounded
// by it.
func NewPasswordValidatorWithTransport(transport http.RoundTripper) *PasswordValidator {
	return &PasswordValidator{
		httpClient: &http.Client{
			Timeout:   breachCheckTimeout,
			Transport: transport,
		},
		commonPasswords: buildCommonPasswordsMap(),
	}
}

// ValidatePassword validates a password against all security requirements
func (v *PasswordValidator) ValidatePassword(password string) (*contracts.PasswordValidationResult, error) {
	result := &contracts.PasswordValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Check minimum length
	if len(password) < MinPasswordLength {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Password must be at least %d characters", MinPasswordLength))
	}

	// Check maximum length
	if len(password) > MaxPasswordLength {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Password must be no more than %d characters", MaxPasswordLength))
	}

	// Check against common passwords
	if v.IsCommonPassword(password) {
		result.Valid = false
		result.Errors = append(result.Errors, "This password is too common and easily guessed")
	}

	// A password the local checks refuse is never sent to the breach check.
	if !result.Valid {
		return result, nil
	}

	// Check against breached passwords (HaveIBeenPwned). The check fails
	// open: when IsBreached returns an error, the password is accepted on the
	// local checks alone and the failure is logged.
	started := v.clock()
	breached, err := v.IsBreached(password)
	if err != nil {
		v.logBreachCheckUnavailable(err, v.clock().Sub(started))
		result.Warnings = append(result.Warnings, "Could not verify password against breach database")
	} else if breached {
		result.Valid = false
		result.Errors = append(result.Errors, "This password has been exposed in a data breach and should not be used")
	}

	return result, nil
}

// IsBreached checks if a password appears in the HaveIBeenPwned database
// Uses k-anonymity: only the first 5 characters of the SHA-1 hash are sent
func (v *PasswordValidator) IsBreached(password string) (bool, error) {
	// Hash the password with SHA-1
	hash := sha1.Sum([]byte(password))
	hexHash := strings.ToUpper(hex.EncodeToString(hash[:]))

	// Split into prefix (5 chars) and suffix (rest)
	prefix := hexHash[:5]
	suffix := hexHash[5:]

	// Query the HaveIBeenPwned API
	url := fmt.Sprintf("https://api.pwnedpasswords.com/range/%s", prefix)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	// Add recommended headers
	req.Header.Set("User-Agent", "PsychicHomily-PasswordCheck")
	req.Header.Set("Add-Padding", "true")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to query pwned passwords: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	if resp.StatusCode != http.StatusOK {
		return false, &breachCheckStatusError{statusCode: resp.StatusCode}
	}

	// Each line of a range response is a record, SUFFIX:COUNT. Lines without
	// that shape are skipped; a body with no record at all is not a range
	// response, so it proves nothing about the password.
	scanner := bufio.NewScanner(resp.Body)
	sawRecord := false
	for scanner.Scan() {
		recordSuffix, ok := parseRangeRecord(scanner.Text())
		if !ok {
			continue
		}
		if strings.EqualFold(recordSuffix, suffix) {
			return true, nil
		}
		sawRecord = true
	}

	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return false, errBreachCheckBadBody
		}
		return false, fmt.Errorf("failed to read response: %w", err)
	}
	if !sawRecord {
		return false, errBreachCheckBadBody
	}

	return false, nil
}

// parseRangeRecord returns the suffix of line when line is a range record:
// exactly rangeRecordSuffixLength hex characters, a colon, and one or more
// decimal digits, with nothing else on the line.
func parseRangeRecord(line string) (string, bool) {
	recordSuffix, count, found := strings.Cut(line, ":")
	if !found || len(recordSuffix) != rangeRecordSuffixLength || count == "" {
		return "", false
	}
	for _, c := range recordSuffix {
		if !isHexDigit(c) {
			return "", false
		}
	}
	for _, c := range count {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	return recordSuffix, true
}

func isHexDigit(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// logBreachCheckUnavailable writes password_breach_check_unavailable at Warn
// unless this validator wrote it less than breachCheckUnavailableLogInterval
// ago. The line carries the failure's reason class and the check's elapsed
// time only: err's text can hold the request URL, which carries the hash
// prefix, so it is never logged.
func (v *PasswordValidator) logBreachCheckUnavailable(err error, elapsed time.Duration) {
	v.unavailableLogMu.Lock()
	now := v.clock()
	if now.Sub(v.unavailableLoggedAt) < breachCheckUnavailableLogInterval {
		v.unavailableLogMu.Unlock()
		return
	}
	v.unavailableLoggedAt = now
	v.unavailableLogMu.Unlock()

	v.log().Warn("password_breach_check_unavailable",
		"reason", breachCheckFailureReason(err),
		"elapsed_ms", elapsed.Milliseconds(),
	)
}

// breachCheckFailureReason classifies a failed breach check as
// "status_<code>" for a non-200 response, "bad_body" for a 200 response with
// no range records, "timeout" when a deadline passed before the response was
// read in full, and "network" for every other failure.
func breachCheckFailureReason(err error) string {
	var statusErr *breachCheckStatusError
	if errors.As(err, &statusErr) {
		return fmt.Sprintf("status_%d", statusErr.statusCode)
	}
	if errors.Is(err, errBreachCheckBadBody) {
		return "bad_body"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	return "network"
}

func (v *PasswordValidator) clock() time.Time {
	if v.now != nil {
		return v.now()
	}
	return time.Now()
}

func (v *PasswordValidator) log() *slog.Logger {
	if v.logger != nil {
		return v.logger
	}
	return logger.Default()
}

// IsCommonPassword checks if a password is in the common passwords list
func (v *PasswordValidator) IsCommonPassword(password string) bool {
	// Check lowercase version for case-insensitive matching
	_, exists := v.commonPasswords[strings.ToLower(password)]
	return exists
}

// buildCommonPasswordsMap returns a map of the most common passwords
// This is a subset of the top 10k most common passwords
func buildCommonPasswordsMap() map[string]bool {
	// Top 1000 most common passwords (lowercase for case-insensitive matching)
	// This list is derived from various breach compilations
	commonPasswords := []string{
		"123456", "password", "123456789", "12345678", "12345", "1234567", "1234567890",
		"qwerty", "abc123", "111111", "123123", "admin", "letmein", "welcome",
		"monkey", "dragon", "master", "1234", "login", "sunshine", "princess",
		"qwertyuiop", "solo", "passw0rd", "starwars", "121212", "654321", "password1",
		"password123", "michael", "shadow", "superman", "qazwsx", "ashley", "bailey",
		"iloveyou", "trustno1", "000000", "football", "baseball", "qwerty123", "killer",
		"pepper", "joshua", "hunter", "cheese", "whatever", "martin", "ginger",
		"soccer", "batman", "andrew", "jordan", "matrix", "thomas", "123qwe",
		"summer", "internet", "service", "canada", "hello", "ranger", "harley",
		"passpass", "george", "banana", "computer", "corvette", "maggie", "merlin",
		"peanut", "cookie", "nicole", "guitar", "chicken", "buster", "golfer",
		"diamond", "michelle", "jennifer", "jessica", "hannah", "amanda", "chocolate",
		"jackson", "austin", "chelsea", "purple", "orange", "camaro", "maverick",
		"samantha", "charlie", "midnight", "justin", "dallas", "william", "brandon",
		"matthew", "anthony", "robert", "access", "yankees", "dallas", "thunder",
		"taylor", "muffin", "jasmine", "creative", "coffee", "silver", "secret",
		"fuckoff", "fuckyou", "asshole", "sexy", "hottie", "lovely", "biteme",
		"snoopy", "scooter", "donald", "yankee", "gators", "tigers", "steelers",
		"eagles", "cowboys", "packers", "redsox", "ravens", "broncos", "giants",
		"dolphins", "falcon", "spartan", "badger", "phoenix", "panther", "warrior",
		"password12", "password2", "password3", "pass123", "pass1234", "test123",
		"test1234", "testing", "testing123", "qwerty1", "qwerty12", "abc1234",
		"abcd1234", "aaaa", "aaaaaa", "aaaaaaaa", "1111", "11111", "1111111",
		"11111111", "222222", "333333", "444444", "555555", "666666", "777777",
		"888888", "999999", "147258369", "123321", "321321", "102030", "112233",
		"123654", "654123", "789456123", "159357", "357159", "147852", "258369",
		"asdfgh", "asdfghjkl", "zxcvbnm", "zxcvbn", "poiuytrewq", "mnbvcxz",
		"1q2w3e", "1q2w3e4r", "1q2w3e4r5t", "2wsx3edc", "1qaz2wsx", "qazwsxedc",
		"administrator", "admin123", "admin1234", "root", "root123", "toor",
		"changeme", "default", "letmein123", "welcome1", "welcome123",
		"p@ssw0rd", "p@ssword", "pa$$word", "pa$$w0rd", "passw0rd123",
		"guest", "guest123", "user", "user123", "demo", "demo123",
		"winter", "spring", "summer", "autumn", "january", "february",
		"monday", "friday", "sunday", "password!", "password!!", "password1!",
		"qwerty!@", "asdf!@#$", "zxcv!@#$", "qwerasdf", "zaqwsxcde",
	}

	passwordMap := make(map[string]bool, len(commonPasswords))
	for _, pw := range commonPasswords {
		passwordMap[pw] = true
	}

	return passwordMap
}

// CalculatePasswordStrength returns a strength score (0-100) based on password characteristics
// This can be used by frontend for the strength meter
func CalculatePasswordStrength(password string) int {
	score := 0
	length := len(password)

	// Length scoring (up to 40 points)
	if length >= 12 {
		score += 20
	}
	if length >= 16 {
		score += 10
	}
	if length >= 20 {
		score += 10
	}

	// Character variety scoring (up to 40 points)
	hasLower := false
	hasUpper := false
	hasDigit := false
	hasSpecial := false

	for _, char := range password {
		switch {
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= '0' && char <= '9':
			hasDigit = true
		default:
			hasSpecial = true
		}
	}

	if hasLower {
		score += 10
	}
	if hasUpper {
		score += 10
	}
	if hasDigit {
		score += 10
	}
	if hasSpecial {
		score += 10
	}

	// Entropy bonus (up to 20 points)
	// Simple approximation: longer passwords with more variety get bonus points
	uniqueChars := make(map[rune]bool)
	for _, char := range password {
		uniqueChars[char] = true
	}
	entropyRatio := float64(len(uniqueChars)) / float64(length)
	if entropyRatio > 0.5 && length >= 12 {
		score += 10
	}
	if entropyRatio > 0.7 && length >= 16 {
		score += 10
	}

	if score > 100 {
		score = 100
	}

	return score
}

// GetStrengthLabel returns a human-readable strength label
func GetStrengthLabel(score int) string {
	switch {
	case score < 30:
		return "Weak"
	case score < 50:
		return "Fair"
	case score < 70:
		return "Good"
	case score < 90:
		return "Strong"
	default:
		return "Excellent"
	}
}
