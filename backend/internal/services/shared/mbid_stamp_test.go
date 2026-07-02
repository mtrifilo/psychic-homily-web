package shared

import "testing"

func TestNotifyArtistMBIDStamped(t *testing.T) {
	t.Cleanup(func() { OnArtistMBIDStamped = nil })

	var got []uint
	OnArtistMBIDStamped = func(id uint) { got = append(got, id) }

	NotifyArtistMBIDStamped(42)
	NotifyArtistMBIDStamped(99)

	if len(got) != 2 || got[0] != 42 || got[1] != 99 {
		t.Fatalf("got %v, want [42 99]", got)
	}

	OnArtistMBIDStamped = nil
	NotifyArtistMBIDStamped(1)
	if len(got) != 2 {
		t.Fatalf("nil hook should be no-op, got %v", got)
	}
}
