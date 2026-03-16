package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTimezoneForState_KnownStates(t *testing.T) {
	assert.Equal(t, "America/Phoenix", GetTimezoneForState("AZ"))
	assert.Equal(t, "America/Los_Angeles", GetTimezoneForState("CA"))
	assert.Equal(t, "America/Los_Angeles", GetTimezoneForState("NV"))
	assert.Equal(t, "America/Denver", GetTimezoneForState("CO"))
	assert.Equal(t, "America/Denver", GetTimezoneForState("NM"))
	assert.Equal(t, "America/Chicago", GetTimezoneForState("IL"))
	assert.Equal(t, "America/Chicago", GetTimezoneForState("TX"))
	assert.Equal(t, "America/New_York", GetTimezoneForState("NY"))
}

func TestGetTimezoneForState_CaseInsensitive(t *testing.T) {
	assert.Equal(t, "America/Phoenix", GetTimezoneForState("az"))
	assert.Equal(t, "America/Los_Angeles", GetTimezoneForState("ca"))
	assert.Equal(t, "America/Phoenix", GetTimezoneForState("Az"))
}

func TestGetTimezoneForState_UnknownDefaultsToPhoenix(t *testing.T) {
	assert.Equal(t, "America/Phoenix", GetTimezoneForState("XX"))
	assert.Equal(t, "America/Phoenix", GetTimezoneForState(""))
}
