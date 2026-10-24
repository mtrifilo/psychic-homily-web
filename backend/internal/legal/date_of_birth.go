package legal

import (
	"errors"
	"time"
)

// DateOfBirthLayout is the one form a date of birth travels in: a calendar
// date, "YYYY-MM-DD", with no time and no zone.
const DateOfBirthLayout = "2006-01-02"

// MaxPlausibleAge bounds how long ago a date of birth may be. A date older
// than this is a typing slip (a year of 1066), not an age.
const MaxPlausibleAge = 150

// ErrDateOfBirthInvalid is the refusal for a value that is not a calendar date
// in DateOfBirthLayout, a date after the day it is submitted on, or a date more
// than MaxPlausibleAge years before it.
var ErrDateOfBirthInvalid = errors.New("date of birth is not a plausible calendar date")

// ParseDateOfBirth reads value as a date of birth submitted at now. The result
// is that date at midnight UTC. A date that does not exist (February 30, or
// February 29 of a year that is not a leap year) is refused, because
// time.Parse refuses it rather than rolling it over.
func ParseDateOfBirth(value string, now time.Time) (time.Time, error) {
	dob, err := time.Parse(DateOfBirthLayout, value)
	if err != nil {
		return time.Time{}, ErrDateOfBirthInvalid
	}
	today := utcDate(now)
	if dob.After(today) {
		return time.Time{}, ErrDateOfBirthInvalid
	}
	if AgeOn(dob, now) > MaxPlausibleAge {
		return time.Time{}, ErrDateOfBirthInvalid
	}
	return dob, nil
}

// AgeOn is the number of whole years between dob and the UTC calendar date of
// now: the year count goes up on the birthday itself, never the day before.
// Someone born on February 29 turns a year older on March 1 in a year with no
// February 29, the later of the two days that could stand for it.
func AgeOn(dob, now time.Time) int {
	today := utcDate(now)
	birth := utcDate(dob)
	age := today.Year() - birth.Year()
	if today.Month() < birth.Month() || (today.Month() == birth.Month() && today.Day() < birth.Day()) {
		age--
	}
	return age
}

func utcDate(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
