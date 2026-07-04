package shared

import "testing"

func TestNotifyRadioArtistNameRematch_NilHookIsNoop(t *testing.T) {
	t.Cleanup(func() { OnRadioArtistNameRematch = nil })
	OnRadioArtistNameRematch = nil
	NotifyRadioArtistNameRematch("Boy Harsher") // must not panic
}

func TestNotifyRadioArtistNameRematch_InvokesHook(t *testing.T) {
	t.Cleanup(func() { OnRadioArtistNameRematch = nil })
	var got []string
	OnRadioArtistNameRematch = func(name string) { got = append(got, name) }
	NotifyRadioArtistNameRematch("Metric")
	NotifyRadioArtistNameRematch("")
	if len(got) != 1 || got[0] != "Metric" {
		t.Fatalf("got %v, want [Metric]", got)
	}
}
