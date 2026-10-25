package catalog

import (
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// changedShowColumns returns the entries of updates that would change the
// stored row. An entry whose value the row already holds is dropped, so a save
// that restates the row reaches GORM with an empty map, writes nothing, and
// leaves updated_at where it was. updated_at is the sitemap's lastmod for a
// show, so it moves only when a column actually changes.
//
// The comparison is against the stored value exactly, not against how the page
// renders it: writing "" over NULL is a change, because it changes the row.
// Any column showColumnHolds does not recognise is kept, so a column added to
// showUpdatesToMap and forgotten here still writes; the cost of forgetting is a
// spurious updated_at move, never a lost edit.
func changedShowColumns(show *catalogm.Show, updates map[string]interface{}) map[string]interface{} {
	changed := make(map[string]interface{}, len(updates))
	for column, value := range updates {
		if !showColumnHolds(show, column, value) {
			changed[column] = value
		}
	}
	return changed
}

// showColumnHolds reports whether writing value to column would leave the
// stored row as it is. The value types are the ones showUpdatesToMap puts in
// the map; a value of any other type reports false.
func showColumnHolds(show *catalogm.Show, column string, value interface{}) bool {
	switch column {
	case "title":
		v, ok := value.(string)
		return ok && v == show.Title
	case "event_date":
		v, ok := value.(time.Time)
		return ok && sameStoredInstant(v, show.EventDate)
	case "doors_at":
		return timePtrHolds(value, show.DoorsAt)
	case "music_at":
		return timePtrHolds(value, show.MusicAt)
	case "city":
		return stringHolds(value, show.City)
	case "state":
		return stringHolds(value, show.State)
	case "age_requirement":
		return stringHolds(value, show.AgeRequirement)
	case "description":
		return stringHolds(value, show.Description)
	case "ticket_url":
		return stringHolds(value, show.TicketURL)
	case "price":
		return floatPtrHolds(value, show.Price)
	case "door_price":
		return floatPtrHolds(value, show.DoorPrice)
	case "image_url":
		return stringPtrHolds(value, show.ImageURL)
	}
	return false
}

// sameStoredInstant compares an incoming instant with one read back from a
// TIMESTAMPTZ column, which keeps microseconds. Anything finer in the incoming
// value is lost on write, so instants less than a microsecond apart are the
// same stored value.
func sameStoredInstant(incoming, stored time.Time) bool {
	d := incoming.Sub(stored)
	return d > -time.Microsecond && d < time.Microsecond
}

// timePtrHolds compares a nullable time entry (a typed nil means clear) with
// the stored nullable column.
func timePtrHolds(value interface{}, stored *time.Time) bool {
	v, ok := value.(*time.Time)
	if !ok {
		return false
	}
	if v == nil || stored == nil {
		return v == nil && stored == nil
	}
	return sameStoredInstant(*v, *stored)
}

// stringHolds compares a plain string entry with a nullable text column. The
// entry always writes a non-NULL value, so it holds only when the column is
// non-NULL and equal.
func stringHolds(value interface{}, stored *string) bool {
	v, ok := value.(string)
	return ok && stored != nil && *stored == v
}

// stringPtrHolds compares a nullable string entry (a typed nil means NULL) with
// the stored nullable column.
func stringPtrHolds(value interface{}, stored *string) bool {
	v, ok := value.(*string)
	if !ok {
		return false
	}
	if v == nil || stored == nil {
		return v == nil && stored == nil
	}
	return *v == *stored
}

// floatPtrHolds compares a nullable price entry (a typed nil means clear) with
// the stored DECIMAL(10,2) column. An incoming value with more precision than
// the column keeps reports false and writes; the column rounds it.
func floatPtrHolds(value interface{}, stored *float64) bool {
	v, ok := value.(*float64)
	if !ok {
		return false
	}
	if v == nil || stored == nil {
		return v == nil && stored == nil
	}
	return *v == *stored
}
