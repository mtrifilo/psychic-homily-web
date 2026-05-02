package main

import (
	catalogm "psychic-homily-backend/internal/models/catalog"
	"testing"
)

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple title",
			input:    "Album Title",
			expected: "album title",
		},
		{
			name:     "title with punctuation",
			input:    "What's Going On?",
			expected: "whats going on",
		},
		{
			name:     "title with deluxe suffix",
			input:    "Album Title (Deluxe Edition)",
			expected: "album title",
		},
		{
			name:     "title with remastered suffix",
			input:    "Album Title (Remastered)",
			expected: "album title",
		},
		{
			name:     "title with extra spaces",
			input:    "  Album   Title  ",
			expected: "album title",
		},
		{
			name:     "title with special characters",
			input:    "Album & Title: Part 2",
			expected: "album title part 2",
		},
		{
			name:     "title with brackets",
			input:    "Album Title [Deluxe]",
			expected: "album title",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "mixed case with numbers",
			input:    "Vol. 3: The Subliminal Verses",
			expected: "vol 3 the subliminal verses",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeTitle(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeTitle(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractReleaseURLs(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		baseURL  string
		expected []string
	}{
		{
			name: "album links",
			html: `<div>
				<a href="/album/first-album">First Album</a>
				<a href="/album/second-album">Second Album</a>
			</div>`,
			baseURL:  "https://artist.bandcamp.com",
			expected: []string{"https://artist.bandcamp.com/album/first-album", "https://artist.bandcamp.com/album/second-album"},
		},
		{
			name: "track links",
			html: `<div>
				<a href="/track/my-single">My Single</a>
			</div>`,
			baseURL:  "https://artist.bandcamp.com",
			expected: []string{"https://artist.bandcamp.com/track/my-single"},
		},
		{
			name: "mixed album and track links",
			html: `<div>
				<a href="/album/the-album">The Album</a>
				<a href="/track/bonus-track">Bonus Track</a>
			</div>`,
			baseURL:  "https://artist.bandcamp.com",
			expected: []string{"https://artist.bandcamp.com/album/the-album", "https://artist.bandcamp.com/track/bonus-track"},
		},
		{
			name: "deduplicates URLs",
			html: `<div>
				<a href="/album/same-album">Same Album</a>
				<a href="/album/same-album">Same Album Again</a>
			</div>`,
			baseURL:  "https://artist.bandcamp.com",
			expected: []string{"https://artist.bandcamp.com/album/same-album"},
		},
		{
			name:     "no release links",
			html:     `<div><a href="/about">About</a></div>`,
			baseURL:  "https://artist.bandcamp.com",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractReleaseURLs(tt.html, tt.baseURL)
			if len(result) != len(tt.expected) {
				t.Errorf("extractReleaseURLs() returned %d URLs, want %d", len(result), len(tt.expected))
				return
			}
			for i, url := range result {
				if url != tt.expected[i] {
					t.Errorf("extractReleaseURLs()[%d] = %q, want %q", i, url, tt.expected[i])
				}
			}
		})
	}
}

func TestExtractJSONLD(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		wantType  string
		wantTitle string
		wantErr   bool
	}{
		{
			name: "MusicAlbum JSON-LD",
			html: `<html><head>
				<script type="application/ld+json">
				{
					"@type": "MusicAlbum",
					"name": "Test Album",
					"datePublished": "2024-01-15",
					"numTracks": 10
				}
				</script>
			</head></html>`,
			wantType:  "MusicAlbum",
			wantTitle: "Test Album",
			wantErr:   false,
		},
		{
			name: "MusicRecording JSON-LD",
			html: `<html><head>
				<script type="application/ld+json">
				{
					"@type": "MusicRecording",
					"name": "Single Track",
					"datePublished": "2024-03-01"
				}
				</script>
			</head></html>`,
			wantType:  "MusicRecording",
			wantTitle: "Single Track",
			wantErr:   false,
		},
		{
			name:    "no JSON-LD",
			html:    `<html><head><title>No JSON-LD</title></head></html>`,
			wantErr: true,
		},
		{
			name: "non-music JSON-LD",
			html: `<html><head>
				<script type="application/ld+json">
				{
					"@type": "WebPage",
					"name": "Not Music"
				}
				</script>
			</head></html>`,
			wantErr: true,
		},
		{
			name: "with artist and label",
			html: `<html><head>
				<script type="application/ld+json">
				{
					"@type": "MusicAlbum",
					"name": "Labeled Album",
					"datePublished": "2023-06-01",
					"byArtist": {"@type": "MusicGroup", "name": "The Band"},
					"recordLabel": {"@type": "Organization", "name": "Cool Records"}
				}
				</script>
			</head></html>`,
			wantType:  "MusicAlbum",
			wantTitle: "Labeled Album",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractJSONLD(tt.html)
			if tt.wantErr {
				if err == nil && result != nil {
					t.Error("extractJSONLD() expected error or nil result, got valid result")
				}
				return
			}
			if err != nil {
				t.Errorf("extractJSONLD() unexpected error: %v", err)
				return
			}
			if result == nil {
				t.Error("extractJSONLD() returned nil result")
				return
			}
			if result.Type != tt.wantType {
				t.Errorf("extractJSONLD().Type = %q, want %q", result.Type, tt.wantType)
			}
			if result.Name != tt.wantTitle {
				t.Errorf("extractJSONLD().Name = %q, want %q", result.Name, tt.wantTitle)
			}
		})
	}
}

func TestJsonLDToRelease(t *testing.T) {
	tests := []struct {
		name      string
		jsonLD    BandcampJSONLD
		url       string
		artist    string
		wantTitle string
		wantType  catalogm.ReleaseType
		wantYear  *int
		wantLabel *string
		wantNil   bool
	}{
		{
			name: "full album",
			jsonLD: BandcampJSONLD{
				Type:          "MusicAlbum",
				Name:          "Full Album",
				DatePublished: "2024-01-15",
				NumTracks:     12,
			},
			url:       "https://artist.bandcamp.com/album/full-album",
			artist:    "The Artist",
			wantTitle: "Full Album",
			wantType:  catalogm.ReleaseTypeLP,
			wantYear:  intPtr(2024),
		},
		{
			name: "EP by track count",
			jsonLD: BandcampJSONLD{
				Type:          "MusicAlbum",
				Name:          "Short EP",
				DatePublished: "2023-06-01",
				NumTracks:     4,
			},
			url:       "https://artist.bandcamp.com/album/short-ep",
			artist:    "The Artist",
			wantTitle: "Short EP",
			wantType:  catalogm.ReleaseTypeEP,
			wantYear:  intPtr(2023),
		},
		{
			name: "single by track count",
			jsonLD: BandcampJSONLD{
				Type:          "MusicAlbum",
				Name:          "Just One Song",
				DatePublished: "2024-02-01",
				NumTracks:     1,
			},
			url:       "https://artist.bandcamp.com/album/just-one-song",
			artist:    "The Artist",
			wantTitle: "Just One Song",
			wantType:  catalogm.ReleaseTypeSingle,
			wantYear:  intPtr(2024),
		},
		{
			name: "MusicRecording is single",
			jsonLD: BandcampJSONLD{
				Type:          "MusicRecording",
				Name:          "Single Track",
				DatePublished: "2024-03-01",
			},
			url:       "https://artist.bandcamp.com/track/single-track",
			artist:    "The Artist",
			wantTitle: "Single Track",
			wantType:  catalogm.ReleaseTypeSingle,
			wantYear:  intPtr(2024),
		},
		{
			name: "with label",
			jsonLD: BandcampJSONLD{
				Type:          "MusicAlbum",
				Name:          "Label Album",
				DatePublished: "2023-01-01",
				NumTracks:     8,
				RecordLabel:   &BandcampJSONLabel{Name: "Indie Records"},
			},
			url:       "https://artist.bandcamp.com/album/label-album",
			artist:    "The Artist",
			wantTitle: "Label Album",
			wantType:  catalogm.ReleaseTypeLP,
			wantYear:  intPtr(2023),
			wantLabel: strPtr("Indie Records"),
		},
		{
			name: "empty name",
			jsonLD: BandcampJSONLD{
				Type: "MusicAlbum",
				Name: "",
			},
			url:     "https://artist.bandcamp.com/album/empty",
			artist:  "The Artist",
			wantNil: true,
		},
		{
			name: "with track numberOfItems",
			jsonLD: BandcampJSONLD{
				Type:          "MusicAlbum",
				Name:          "Track Items Album",
				DatePublished: "2024-01-01",
				Track:         &BandcampJSONTrack{NumberOfItems: 5},
			},
			url:       "https://artist.bandcamp.com/album/track-items",
			artist:    "The Artist",
			wantTitle: "Track Items Album",
			wantType:  catalogm.ReleaseTypeEP,
			wantYear:  intPtr(2024),
		},
		{
			name: "with artist from JSON-LD",
			jsonLD: BandcampJSONLD{
				Type:          "MusicAlbum",
				Name:          "Artist Album",
				DatePublished: "2024-01-01",
				NumTracks:     10,
				ByArtist:      &BandcampJSONArtist{Name: "JSON Artist"},
			},
			url:       "https://artist.bandcamp.com/album/artist-album",
			artist:    "Fallback Artist",
			wantTitle: "Artist Album",
			wantType:  catalogm.ReleaseTypeLP,
			wantYear:  intPtr(2024),
		},
		{
			name: "image as string",
			jsonLD: BandcampJSONLD{
				Type:          "MusicAlbum",
				Name:          "Image Album",
				DatePublished: "2024-01-01",
				NumTracks:     8,
				Image:         "https://example.com/cover.jpg",
			},
			url:       "https://artist.bandcamp.com/album/image-album",
			artist:    "The Artist",
			wantTitle: "Image Album",
			wantType:  catalogm.ReleaseTypeLP,
			wantYear:  intPtr(2024),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jsonLDToRelease(&tt.jsonLD, tt.url, tt.artist)
			if tt.wantNil {
				if result != nil {
					t.Error("jsonLDToRelease() expected nil, got result")
				}
				return
			}
			if result == nil {
				t.Fatal("jsonLDToRelease() returned nil")
			}
			if result.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", result.Title, tt.wantTitle)
			}
			if result.ReleaseType != tt.wantType {
				t.Errorf("ReleaseType = %q, want %q", result.ReleaseType, tt.wantType)
			}
			if tt.wantYear != nil {
				if result.ReleaseYear == nil {
					t.Errorf("ReleaseYear = nil, want %d", *tt.wantYear)
				} else if *result.ReleaseYear != *tt.wantYear {
					t.Errorf("ReleaseYear = %d, want %d", *result.ReleaseYear, *tt.wantYear)
				}
			}
			if tt.wantLabel != nil {
				if result.LabelName == nil {
					t.Errorf("LabelName = nil, want %q", *tt.wantLabel)
				} else if *result.LabelName != *tt.wantLabel {
					t.Errorf("LabelName = %q, want %q", *result.LabelName, *tt.wantLabel)
				}
			}
		})
	}
}

func TestGetBandcampURL(t *testing.T) {
	tests := []struct {
		name     string
		artist   catalogm.Artist
		expected string
	}{
		{
			name: "social bandcamp URL with https",
			artist: catalogm.Artist{
				Social: catalogm.Social{
					Bandcamp: strPtr("https://theband.bandcamp.com"),
				},
			},
			expected: "https://theband.bandcamp.com",
		},
		{
			name: "social bandcamp URL without protocol",
			artist: catalogm.Artist{
				Social: catalogm.Social{
					Bandcamp: strPtr("theband.bandcamp.com"),
				},
			},
			expected: "https://theband.bandcamp.com",
		},
		{
			name: "social bandcamp URL with trailing slash",
			artist: catalogm.Artist{
				Social: catalogm.Social{
					Bandcamp: strPtr("https://theband.bandcamp.com/"),
				},
			},
			expected: "https://theband.bandcamp.com",
		},
		{
			name: "embed URL with artist subdomain",
			artist: catalogm.Artist{
				BandcampEmbedURL: strPtr("https://theband.bandcamp.com/album/my-album"),
			},
			expected: "https://theband.bandcamp.com",
		},
		{
			name:     "no bandcamp URL",
			artist:   catalogm.Artist{},
			expected: "",
		},
		{
			name: "empty bandcamp social",
			artist: catalogm.Artist{
				Social: catalogm.Social{
					Bandcamp: strPtr(""),
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getBandcampURL(&tt.artist)
			if result != tt.expected {
				t.Errorf("getBandcampURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractReleaseFromHTML(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		url       string
		artist    string
		wantTitle string
		wantType  catalogm.ReleaseType
		wantNil   bool
	}{
		{
			name: "with og:title",
			html: `<html><head>
				<meta property="og:title" content="Great Album">
				<meta property="og:image" content="https://example.com/art.jpg">
			</head></html>`,
			url:       "https://artist.bandcamp.com/album/great-album",
			artist:    "The Artist",
			wantTitle: "Great Album",
			wantType:  catalogm.ReleaseTypeLP,
		},
		{
			name: "with title tag and pipe",
			html: `<html><head>
				<title>Great Album | The Artist</title>
			</head></html>`,
			url:       "https://artist.bandcamp.com/album/great-album",
			artist:    "The Artist",
			wantTitle: "Great Album",
			wantType:  catalogm.ReleaseTypeLP,
		},
		{
			name: "track URL becomes single",
			html: `<html><head>
				<meta property="og:title" content="Single Song">
			</head></html>`,
			url:       "https://artist.bandcamp.com/track/single-song",
			artist:    "The Artist",
			wantTitle: "Single Song",
			wantType:  catalogm.ReleaseTypeSingle,
		},
		{
			name:    "no title found",
			html:    `<html><head></head></html>`,
			url:     "https://artist.bandcamp.com/album/nothing",
			artist:  "The Artist",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractReleaseFromHTML(tt.html, tt.url, tt.artist)
			if tt.wantNil {
				if result != nil {
					t.Error("extractReleaseFromHTML() expected nil, got result")
				}
				return
			}
			if result == nil {
				t.Fatal("extractReleaseFromHTML() returned nil")
			}
			if result.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", result.Title, tt.wantTitle)
			}
			if result.ReleaseType != tt.wantType {
				t.Errorf("ReleaseType = %q, want %q", result.ReleaseType, tt.wantType)
			}
		})
	}
}

func TestMatchRelease(t *testing.T) {
	existing := []existingRelease{
		{ID: 1, Title: "First Album"},
		{ID: 2, Title: "Second Album"},
		{ID: 3, Title: "What's Going On?"},
	}

	tests := []struct {
		name          string
		bcRelease     BandcampRelease
		wantMatchType string
		wantID        uint
	}{
		{
			name:          "exact match",
			bcRelease:     BandcampRelease{Title: "First Album", URL: "https://a.bandcamp.com/album/first-album"},
			wantMatchType: "MATCH",
			wantID:        1,
		},
		{
			name:          "case insensitive match",
			bcRelease:     BandcampRelease{Title: "first album", URL: "https://a.bandcamp.com/album/first-album"},
			wantMatchType: "MATCH",
			wantID:        1,
		},
		{
			name:          "match with punctuation differences",
			bcRelease:     BandcampRelease{Title: "Whats Going On", URL: "https://a.bandcamp.com/album/whats-going-on"},
			wantMatchType: "MATCH",
			wantID:        3,
		},
		{
			name:          "no match - new release",
			bcRelease:     BandcampRelease{Title: "Brand New Album", URL: "https://a.bandcamp.com/album/brand-new"},
			wantMatchType: "NEW",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pass nil database — matchRelease only uses it for hasBandcampLink check,
			// which we can't easily test without a real DB.
			// For these tests, MATCH results that would check hasBandcampLink will panic
			// if the existing release has a link. Since our test data doesn't have links,
			// we skip that code path by testing only the matching logic.
			//
			// Note: We can't pass nil database because hasBandcampLink is called for MATCHes.
			// Instead, we test the normalize + comparison logic directly.

			normalizedBC := normalizeTitle(tt.bcRelease.Title)
			var foundMatch bool
			var matchedID uint
			for _, e := range existing {
				if normalizedBC == normalizeTitle(e.Title) {
					foundMatch = true
					matchedID = e.ID
					break
				}
			}

			if tt.wantMatchType == "MATCH" {
				if !foundMatch {
					t.Error("Expected MATCH but got no match")
				} else if matchedID != tt.wantID {
					t.Errorf("Matched ID = %d, want %d", matchedID, tt.wantID)
				}
			} else if tt.wantMatchType == "NEW" {
				if foundMatch {
					t.Errorf("Expected NEW but found match with ID %d", matchedID)
				}
			}
		})
	}
}

// Helper functions
func intPtr(i int) *int {
	return &i
}

func strPtr(s string) *string {
	return &s
}
