#!/usr/bin/env python3
"""Regenerate cities.tsv from GeoNames + the US Census CBSA delineation file.

Output columns (tab-separated, one populated place per row):
    name, asciiname, countryCode, admin1, population, latitude, longitude,
    timezone, cbsaCode, cbsaName

cbsaCode/cbsaName are the US Census CBSA (metro/micro area) the place's COUNTY
rolls up to — set for US rows only. They are what makes a borough/suburb share
one metro key with its core city (Brooklyn → New York-Newark-Jersey City,
Pasadena CA → Los Angeles-Long Beach-Anaheim). Joined offline via county FIPS:
GeoNames gives admin1 (2-letter state) + admin2 (3-digit county FIPS); we prefix
the state FIPS to get the 5-digit county code the Census crosswalk keys on.

Inputs (download first — see README.md):
    cities15000.txt   GeoNames populated places (pop >= 15,000)
    list1.xlsx        Census/OMB CBSA delineation file (county -> CBSA)

Requires: pandas + openpyxl (pip install pandas openpyxl).
Usage: python3 gen_cities.py cities15000.txt list1.xlsx > cities.tsv
"""
import sys
import pandas as pd

# GeoNames 2-letter US state code -> FIPS state code (2-digit).
STATE_FIPS = {
    'AL': '01', 'AK': '02', 'AZ': '04', 'AR': '05', 'CA': '06', 'CO': '08',
    'CT': '09', 'DE': '10', 'DC': '11', 'FL': '12', 'GA': '13', 'HI': '15',
    'ID': '16', 'IL': '17', 'IN': '18', 'IA': '19', 'KS': '20', 'KY': '21',
    'LA': '22', 'ME': '23', 'MD': '24', 'MA': '25', 'MI': '26', 'MN': '27',
    'MS': '28', 'MO': '29', 'MT': '30', 'NE': '31', 'NV': '32', 'NH': '33',
    'NJ': '34', 'NM': '35', 'NY': '36', 'NC': '37', 'ND': '38', 'OH': '39',
    'OK': '40', 'OR': '41', 'PA': '42', 'RI': '44', 'SC': '45', 'SD': '46',
    'TN': '47', 'TX': '48', 'UT': '49', 'VT': '50', 'VA': '51', 'WA': '53',
    'WV': '54', 'WI': '55', 'WY': '56', 'PR': '72',
}


def load_crosswalk(xlsx_path):
    """5-digit county FIPS -> (cbsa_code, cbsa_title)."""
    df = pd.read_excel(xlsx_path, skiprows=2)  # 2 title rows precede the header
    xw = {}
    for _, r in df.iterrows():
        try:
            state = str(int(r['FIPS State Code'])).zfill(2)
            county = str(int(r['FIPS County Code'])).zfill(3)
        except (ValueError, TypeError):
            continue
        xw[state + county] = (str(int(r['CBSA Code'])), str(r['CBSA Title']).strip())
    return xw


def main(cities_path, xlsx_path):
    xw = load_crosswalk(xlsx_path)
    with open(cities_path, encoding='utf-8') as f:
        for line in f:
            p = line.rstrip('\n').split('\t')
            if len(p) < 19:
                continue
            name, ascii_, country = p[1], p[2], p[8]
            admin1, admin2, pop = p[10], p[11], p[14]
            lat, lng, tz = p[4], p[5], p[17]
            cbsa_code = cbsa_name = ''
            if country == 'US':
                sf = STATE_FIPS.get(admin1.upper())
                if sf and admin2:
                    hit = xw.get(sf + admin2.zfill(3))
                    if hit:
                        cbsa_code, cbsa_name = hit
            print('\t'.join([name, ascii_, country, admin1, pop, lat, lng, tz, cbsa_code, cbsa_name]))


if __name__ == '__main__':
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    main(sys.argv[1], sys.argv[2])
