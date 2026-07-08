# Embedded geocoding data

`cities.tsv` and `countries.tsv` are derived from the [GeoNames](https://www.geonames.org/)
geographical database, licensed under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).
**© GeoNames, CC BY 4.0** — attribution required when redistributing.

The CBSA columns in `cities.tsv` are joined from the U.S. Census Bureau / OMB
[Core-Based Statistical Area delineation files](https://www.census.gov/geographies/reference-files/time-series/demo/metro-micro/delineation-files.html),
which are **public domain** (U.S. government work).

They are embedded into the binary (`//go:embed`) by `geo.go` and parsed once on
first use to resolve a venue's `(city, state, country)` → `(lat, lng, IANA timezone)`
fully offline (no API, key, or rate limit).

## Files

- **`cities.tsv`** — slimmed from GeoNames `cities1000.txt` (populated places
  with population ≥ 1,000, plus admin seats — PSY-1377, upgraded from cities15000
  so sub-15k towns in split-timezone states resolve to their exact IANA zone
  instead of a state-level fallback). Tab-separated columns:
  `name, asciiname, countryCode, admin1, population, latitude, longitude,
  timezone, cbsaCode, cbsaName`. The last two are the US Census CBSA (metro/micro
  area) the place's county rolls up to — **US rows only**, empty otherwise — and
  are what let boroughs/suburbs share one metro key (Brooklyn → New York-Newark-
  Jersey City; Pasadena CA → Los Angeles-Long Beach-Anaheim). See `gen_cities.py`.
- **`countries.tsv`** — `ISO2 \t country name`, from GeoNames `countryInfo.txt`.
- **`gen_cities.py`** — joins GeoNames + the Census CBSA delineation into
  `cities.tsv` (needs `pandas` + `openpyxl`). The CBSA delineation
  (**public domain**, OMB/Census) is the canonical metro definition.

## Regenerate

```bash
cd /tmp
curl -sL https://download.geonames.org/export/dump/cities1000.zip -o cities1000.zip && unzip -o cities1000.zip
curl -sL https://download.geonames.org/export/dump/countryInfo.txt -o countryInfo.txt
# CBSA delineation (county -> CBSA). Keep the year that matches the committed file
# (see Provenance) unless you intend to refresh the metro definitions too.
curl -sL https://www2.census.gov/programs-surveys/metro-micro/geographies/reference-files/2023/delineation-files/list1_2023.xlsx -o list1.xlsx

pip install pandas openpyxl
python3 "$OLDPWD/internal/services/geo/data/gen_cities.py" cities1000.txt list1.xlsx > cities.tsv

# countries: ISO2, name
grep -v '^#' countryInfo.txt | awk -F'\t' 'NF>=5 && $1!="" {print $1"\t"$5}' > countries.tsv
```

Then move `cities.tsv` / `countries.tsv` into this directory.

## Provenance

The committed `cities.tsv` was generated from:
- **GeoNames `cities1000`** (populated places ≥ 1,000 + admin seats). Some rows have
  `population = 0` — those are GeoNames administrative seats (county seats etc.),
  not data errors; they only win a lookup when they're the sole candidate for a state.
- **US Census/OMB CBSA delineation `list1_2023.xlsx`** (2023 vintage) — pinned so
  the county→metro mapping is reproducible; bump it only to intentionally refresh
  the metro definitions (which also shifts scene/metro keys downstream).

`gen_cities.py` de-duplicates same-name, same-state rows whose IANA timezones
disagree, keeping the higher GeoNames admin rank (a county seat over a mis-placed
duplicate) so the geocoder's max-population pick can't select a wrong-zone row.

Verify a regeneration matches the committed artifact:

```
$ shasum -a 256 cities.tsv
e34387f2c4ea35ba51e8d66a89a048c68ea700a22a5f9eebd2f652b8724e894b  cities.tsv
```
