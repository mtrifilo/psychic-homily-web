package services

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Password validation constants
const (
	MinPasswordLength = 12
	MaxPasswordLength = 128
)

// PasswordValidationResult contains the result of password validation
type PasswordValidationResult struct {
	Valid    bool
	Errors   []string
	Warnings []string
}

// PasswordValidator validates passwords against security requirements
type PasswordValidator struct {
	httpClient      *http.Client
	commonPasswords map[string]bool
}

// NewPasswordValidator creates a new password validator
func NewPasswordValidator() *PasswordValidator {
	return &PasswordValidator{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		commonPasswords: buildCommonPasswordsMap(),
	}
}

// ValidatePassword validates a password against all security requirements
func (v *PasswordValidator) ValidatePassword(password string) (*PasswordValidationResult, error) {
	result := &PasswordValidationResult{
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

	// Check against breached passwords (HaveIBeenPwned)
	breached, err := v.IsBreached(password)
	if err != nil {
		// Log but don't fail - breach check is best effort
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("pwned passwords API returned status %d", resp.StatusCode)
	}

	// Parse response - each line is SUFFIX:COUNT
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) >= 1 && strings.EqualFold(parts[0], suffix) {
			return true, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	return false, nil
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
