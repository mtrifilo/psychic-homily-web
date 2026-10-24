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

// latestUTCOffset is the furthest-ahead offset any place on Earth keeps
// (UTC+14, the Line Islands). The calendar date there is the latest date it
// is anywhere, so a birthday counts from the first moment it is that day
// somewhere.
const latestUTCOffset = 14 * time.Hour

// ErrDateOfBirthInvalid is the refusal for a value that is not a calendar date
// in DateOfBirthLayout, a date after the latest calendar date anywhere at
// submission, or a date that gives an age over MaxPlausibleAge then.
var ErrDateOfBirthInvalid = errors.New("date of birth is not a plausible calendar date")

// ParseDateOfBirth reads value as a date of birth submitted at now. The result
// is that date at midnight UTC. A date that does not exist (February 30, or
// February 29 of a year that is not a leap year) is refused, because
// time.Parse refuses it rather than rolling it over. "Today" is the latest
// calendar date anywhere at now, the date AgeOn counts against.
func ParseDateOfBirth(value string, now time.Time) (time.Time, error) {
	dob, err := time.Parse(DateOfBirthLayout, value)
	if err != nil {
		return time.Time{}, ErrDateOfBirthInvalid
	}
	if dob.After(latestDate(now)) {
		return time.Time{}, ErrDateOfBirthInvalid
	}
	if AgeOn(dob, now) > MaxPlausibleAge {
		return time.Time{}, ErrDateOfBirthInvalid
	}
	return dob, nil
}

// FormatDateOfBirth renders a stored date of birth in DateOfBirthLayout. The
// date is read in the location it carries: a date of birth names a calendar
// day, not an instant, so converting it to another zone could move the day.
func FormatDateOfBirth(dob time.Time) string {
	return dob.Format(DateOfBirthLayout)
}

// AgeOn is the number of whole years between dob and the latest calendar date
// anywhere on Earth at now (the date at UTC+14): someone is a year older from
// the first moment it is their birthday somewhere, so nobody who has reached
// an age anywhere is counted as younger. The count goes up on the birthday
// itself, never the day before. Someone born on February 29 turns a year
// older on March 1 in a year with no February 29, the later of the two days
// that could stand for it.
func AgeOn(dob, now time.Time) int {
	today := latestDate(now)
	birth := utcDate(dob)
	age := today.Year() - birth.Year()
	if today.Month() < birth.Month() || (today.Month() == birth.Month() && today.Day() < birth.Day()) {
		age--
	}
	return age
}

// latestDate is the calendar date at UTC+14 at now, at midnight UTC.
func latestDate(now time.Time) time.Time {
	return utcDate(now.UTC().Add(latestUTCOffset))
}

func utcDate(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
