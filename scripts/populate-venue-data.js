#!/usr/bin/env node

const fs = require("fs");
const path = require("path");
const yaml = require("js-yaml");
const glob = require("glob");
const prompts = require("prompts");

// Load existing venues data
const venuesPath = path.join(process.cwd(), "data", "venues.yaml");
let venuesData = yaml.load(fs.readFileSync(venuesPath, "utf8")) || {};

// Find all show markdown files
const showFiles = glob.sync(
  path.join(process.cwd(), "content", "shows", "*.md")
);

console.log(`Found ${showFiles.length} show files to scan for venues`);

// Track missing venues
const missingVenues = new Map();

// Scan all show files for venue information
showFiles.forEach((filePath) => {
  try {
    // Skip index file
    if (filePath.endsWith("_index.md")) {
      return;
    }

    const content = fs.readFileSync(filePath, "utf8");
    const frontmatterMatch = content.match(/^---\n([\s\S]*?)\n---/);

    if (!frontmatterMatch) {
      console.log(`No frontmatter found in ${filePath}`);
      return;
    }

    const frontmatter = frontmatterMatch[1];
    let venueIds = [];

    // Check for venues array
    const venuesMatch = frontmatter.match(
      /venues:\s*\n((?:\s*-\s*["'].*["']\s*\n)+)/
    );
    if (venuesMatch) {
      const venuesBlock = venuesMatch[1];
      const venueMatches = venuesBlock.matchAll(/-\s*["'](.+?)["']/g);
      for (const match of venueMatches) {
        venueIds.push(match[1]);
      }
    }

    // Check for single venue
    const venueMatch = frontmatter.match(/venue:\s*["'](.+?)["']/);
    if (venueMatch) {
      venueIds.push(venueMatch[1]);
    }

    // Extract city and state
    const cityMatch = frontmatter.match(/city:\s*["'](.+?)["']/);
    const stateMatch = frontmatter.match(/state:\s*["'](.+?)["']/);

    const city = cityMatch ? cityMatch[1] : null;
    const state = stateMatch ? stateMatch[1] : null;

    // Check if venues exist in venues data
    venueIds.forEach((venueId) => {
      if (!venuesData[venueId]) {
        // Store missing venue with city and state if available
        if (!missingVenues.has(venueId)) {
          missingVenues.set(venueId, {
            count: 1,
            city,
            state,
            files: [path.basename(filePath)],
          });
        } else {
          const data = missingVenues.get(venueId);
          data.count++;
          data.files.push(path.basename(filePath));
          // Use city and state if not already set
          if (!data.city && city) data.city = city;
          if (!data.state && state) data.state = state;
          missingVenues.set(venueId, data);
        }
      }
    });
  } catch (error) {
    console.error(`Error processing ${filePath}:`, error);
  }
});

// Define known venue data for common venues
const knownVenueData = {
  "crescent-ballroom": {
    name: "Crescent Ballroom",
    address: "308 N 2nd Ave",
    city: "Phoenix",
    state: "AZ",
    zip: "85003",
    social: {
      instagram: "crescentphx",
      website: "https://www.crescentphx.com",
    },
  },
  "valley-bar": {
    name: "Valley Bar",
    address: "130 N Central Ave",
    city: "Phoenix",
    state: "AZ",
    zip: "85004",
    social: {
      instagram: "valleybarphx",
      website: "https://www.valleybarphx.com",
    },
  },
  "the-rebel-lounge": {
    name: "The Rebel Lounge",
    address: "2303 E Indian School Rd",
    city: "Phoenix",
    state: "AZ",
    zip: "85016",
    social: {
      instagram: "therebellounge",
      website: "https://www.therebellounge.com",
    },
  },
  "club-congress": {
    name: "Club Congress",
    address: "311 E Congress St",
    city: "Tucson",
    state: "AZ",
    zip: "85701",
    social: {
      instagram: "hotelcongress",
      website: "https://hotelcongress.com/music",
    },
  },
  "walter-studios": {
    name: "Walter Studios",
    address: "747 W Roosevelt St",
    city: "Phoenix",
    state: "AZ",
    zip: "85007",
    social: {
      instagram: "walter_studios",
      website: "https://www.walterstudios.art",
    },
  },
  "palo-verde-lounge": {
    name: "Palo Verde Lounge",
    address: "1015 W Broadway Rd",
    city: "Tempe",
    state: "AZ",
    zip: "85282",
    social: {
      instagram: "paloverdebar",
    },
  },
};

async function main() {
  // Display missing venues
  if (missingVenues.size === 0) {
    console.log(
      "\nNo missing venues found! All venues are properly defined in venues.yaml."
    );
    process.exit(0);
  }

  console.log(`\nFound ${missingVenues.size} missing venues:`);
  const sortedMissingVenues = [...missingVenues.entries()].sort(
    (a, b) => b[1].count - a[1].count
  );

  sortedMissingVenues.forEach(([venueId, data], index) => {
    console.log(`${index + 1}. ${venueId} (used in ${data.count} shows)`);
    console.log(
      `   City: ${data.city || "Unknown"}, State: ${data.state || "Unknown"}`
    );
    console.log(
      `   Example files: ${data.files.slice(0, 3).join(", ")}${
        data.files.length > 3 ? "..." : ""
      }`
    );
  });

  const { confirm } = await prompts({
    type: "confirm",
    name: "confirm",
    message: "Add all missing venues to venues.yaml?",
    initial: true,
  });

  if (!confirm) {
    console.log("Operation cancelled. No changes made.");
    process.exit(0);
  }

  // Add missing venues to venues data
  let addedCount = 0;
  for (const [venueId, data] of missingVenues.entries()) {
    // Check if we have known data for this venue
    if (knownVenueData[venueId]) {
      venuesData[venueId] = knownVenueData[venueId];
      console.log(
        `Added with known data: ${venueId} (${knownVenueData[venueId].name})`
      );
    } else {
      // Generate a human-readable name from the venue ID
      const venueName = venueId
        .split("-")
        .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
        .join(" ");

      venuesData[venueId] = {
        name: venueName,
        city: data.city || "",
        state: data.state || "",
      };

      console.log(`Added with basic data: ${venueId} (${venueName})`);
    }

    addedCount++;
  }

  // Write updated venues data back to file
  fs.writeFileSync(venuesPath, yaml.dump(venuesData));

  console.log(`\nSuccess! Added ${addedCount} missing venues to venues.yaml.`);
  console.log(
    "You may want to review the venues.yaml file to add additional details like addresses and social links."
  );
}

main().catch((error) => {
  console.error("Error:", error);
  process.exit(1);
});
