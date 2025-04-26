const yaml = require("js-yaml");
const fs = require("fs");
const path = require("path");

// Convert bands.yaml to JSON
const bandsYaml = fs.readFileSync(
  path.join(__dirname, "data/bands.yaml"),
  "utf8"
);
const bandsJson = yaml.load(bandsYaml);
fs.writeFileSync(
  path.join(__dirname, "psychic-homily-components/src/lib/bands.json"),
  JSON.stringify(bandsJson, null, 2)
);

// Convert venues.yaml to JSON
const venuesYaml = fs.readFileSync(
  path.join(__dirname, "data/venues.yaml"),
  "utf8"
);
const venuesJson = yaml.load(venuesYaml);
fs.writeFileSync(
  path.join(__dirname, "psychic-homily-components/src/lib/venues.json"),
  JSON.stringify(venuesJson, null, 2)
);
