from iso3166 import countries_by_alpha2

def parse(alpha2):
    return countries_by_alpha2[alpha2]

def serialize(country):
    return country.alpha2
