# Embedded geocoding data

`cities.tsv` and `countries.tsv` are derived from the [GeoNames](https://www.geonames.org/)
geographical database, licensed under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).
**© GeoNames, CC BY 4.0** — attribution required when redistributing.

They are embedded into the binary (`//go:embed`) by `geo.go` and parsed once on
first use to resolve a venue's `(city, state, country)` → `(lat, lng, IANA timezone)`
fully offline (no API, key, or rate limit).

## Files

- **`cities.tsv`** — slimmed from GeoNames `cities15000.txt` (populated places
  with population ≥ 15,000; 33,753 rows). Tab-separated columns:
  `name, asciiname, countryCode, admin1, population, latitude, longitude, timezone`.
- **`countries.tsv`** — `ISO2 \t country name`, from GeoNames `countryInfo.txt`.

## Regenerate

```bash
cd /tmp
curl -sL https://download.geonames.org/export/dump/cities15000.zip -o cities15000.zip && unzip -o cities15000.zip
curl -sL https://download.geonames.org/export/dump/countryInfo.txt -o countryInfo.txt
# cities: name, asciiname, country, admin1, population, lat, lng, timezone
awk -F'\t' 'BEGIN{OFS="\t"} {print $2,$3,$9,$11,$15,$5,$6,$18}' cities15000.txt > cities.tsv
# countries: ISO2, name
grep -v '^#' countryInfo.txt | awk -F'\t' 'NF>=5 && $1!="" {print $1"\t"$5}' > countries.tsv
```

Then move `cities.tsv` / `countries.tsv` into this directory.
