package shared

// OnRadioArtistNameRematch schedules a targeted rematch of unmatched radio plays
// whose artist_name resolves to the given name (exact normalized match or alias).
// Set by the service container at startup; nil is a no-op.
var OnRadioArtistNameRematch func(name string)

// OnRadioLabelNameRematch schedules a targeted rematch of unmatched radio plays
// whose label_name resolves to the given name. Set by the service container at
// startup; nil is a no-op.
var OnRadioLabelNameRematch func(name string)

// NotifyRadioArtistNameRematch invokes OnRadioArtistNameRematch when configured.
func NotifyRadioArtistNameRematch(name string) {
	if OnRadioArtistNameRematch != nil && name != "" {
		OnRadioArtistNameRematch(name)
	}
}

// NotifyRadioLabelNameRematch invokes OnRadioLabelNameRematch when configured.
func NotifyRadioLabelNameRematch(name string) {
	if OnRadioLabelNameRematch != nil && name != "" {
		OnRadioLabelNameRematch(name)
	}
}
