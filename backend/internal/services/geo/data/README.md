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

- **`cities.tsv`** — slimmed from GeoNames `cities15000.txt` (populated places
  with population ≥ 15,000). Tab-separated columns:
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
curl -sL https://download.geonames.org/export/dump/cities15000.zip -o cities15000.zip && unzip -o cities15000.zip
curl -sL https://download.geonames.org/export/dump/countryInfo.txt -o countryInfo.txt
# CBSA delineation (county -> CBSA). Bump the year to the latest available.
curl -sL https://www2.census.gov/programs-surveys/metro-micro/geographies/reference-files/2023/delineation-files/list1_2023.xlsx -o list1.xlsx

pip install pandas openpyxl
python3 "$OLDPWD/internal/services/geo/data/gen_cities.py" cities15000.txt list1.xlsx > cities.tsv

# countries: ISO2, name
grep -v '^#' countryInfo.txt | awk -F'\t' 'NF>=5 && $1!="" {print $1"\t"$5}' > countries.tsv
```

Then move `cities.tsv` / `countries.tsv` into this directory.
