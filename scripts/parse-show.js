#!/usr/bin/env node

const fs = require("fs");
const path = require("path");
const yaml = require("js-yaml");
const readline = require("readline");
const Anthropic = require("@anthropic-ai/sdk");
require("dotenv").config();
const prompts = require("prompts");

// Suppress only punycode deprecation warning
process.removeAllListeners("warning");

const anthropic = new Anthropic({
  apiKey: process.env.ANTHROPIC_API_KEY,
});

const rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout,
});

const SYSTEM_PROMPT = `You are a helpful assistant that parses show announcements into structured data. 

Key parsing rules:
1. For Phoenix shows at Linger Longer Lounge, assume Phoenix, AZ as the location
2. Convert all times to 24-hour format (e.g., "8pm" → "20:00")
3. Convert all dates to YYYY-MM-DD format
4. Extract numeric price only (e.g., "$10" → 10)
5. Separate band names when delimited by "•", "and", "&", or commas
6. Preserve exact band name capitalization
7. For missing fields:
   - Default venue to "Phoenix, AZ" if no venue is specified
   - Default city to "Phoenix" and state to "AZ" if no location is specified
   - Default age requirement to "21+" shows with no age requirement specified
   - Default year to 2025 for dates without a specified year. The current year is 2025.

Example inputs and expected parsing:

Input 1:
"Psychic Homily DJ • The Mourning
Friday March 15th at Linger Longer Lounge
8pm • 21+ • $10"

Should parse to:
{
  "date": "2025-03-15",
  "time": "20:00",
  "bands": ["Psychic Homily DJ", "The Mourning"],
  "venue": "Linger Longer Lounge",
  "city": "Phoenix",
  "state": "AZ",
  "price": 10,
  "age_requirement": "21+"
}

Input 2:
"BOJACKS and Psychic Homily DJ
Tuesday April 15 @ Valley Bar
8PM / 21+ / $10"

Should parse to:
{
  "date": "2025-04-15",
  "time": "20:00",
  "bands": ["BOJACKS", "Psychic Homily DJ"],
  "venue": "Valley Bar",
  "city": "Phoenix",
  "state": "AZ",
  "price": 10,
  "age_requirement": "21+"
}

Extract the show details and return them in the specified JSON format. Assume the current or next year for dates depending on context.`;

const prompt = (question) =>
  new Promise((resolve) => rl.question(question, resolve));

async function getShowDetails(text) {
  try {
    console.log("\nSending API request with configuration:");
    const config = {
      model: "claude-3-sonnet-20240229",
      max_tokens: 1024,
      system: SYSTEM_PROMPT,
      messages: [
        {
          role: "user",
          content: `Parse this show announcement into structured data:\n${text}`,
        },
      ],
    };

    console.log("- Model:", config.model);
    console.log("- Input text:", text);

    const response = await anthropic.messages.create(config);

    console.log("\nAPI Response:");
    console.log(JSON.stringify(response, null, 2));

    // Extract just the JSON part using regex
    const jsonMatch = response.content[0].text.match(/\{[\s\S]*\}/);
    if (!jsonMatch) {
      throw new Error("Could not find JSON in response");
    }

    return JSON.parse(jsonMatch[0]);
  } catch (error) {
    console.error("\nAPI Error Details:");
    console.error("- Status:", error.status);
    console.error("- Message:", error.message);
    if (error.response?.body) {
      console.error(
        "- Response body:",
        JSON.stringify(error.response.body, null, 2)
      );
    }
    throw new Error(
      "Failed to parse show details. Please check your input and try again."
    );
  }
}

async function main() {
  const { text } = await prompts({
    type: "text",
    name: "text",
    message:
      "Paste your show announcement text below (press Enter twice when done):",
    multiline: true,
  });

  if (!text) {
    console.log("No input provided. Exiting.");
    process.exit(0);
  }

  // Clean up the input text
  const cleanedText = text
    .split(/\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .join(" ")
    .replace(/(\d+\/\d+\/\d+)([A-Za-z])/, "$1 $2") // Add space after date
    .replace(/\s+/g, " ")
    .trim();

  console.log("\nworking on input:", cleanedText);

  try {
    // Load existing bands data
    const bandsPath = path.join(process.cwd(), "data", "bands.yaml");
    const bandsData = yaml.load(fs.readFileSync(bandsPath, "utf8")) || {};

    // Load existing venues data
    const venuesPath = path.join(process.cwd(), "data", "venues.yaml");
    const venuesData = yaml.load(fs.readFileSync(venuesPath, "utf8")) || {};

    // Parse the show details
    const showDetails = await getShowDetails(cleanedText);

    // Preview the parsed data
    console.log("\nParsed show details:");
    console.log("------------------");
    console.log(`Date: ${showDetails.date}`);
    console.log(`Time: ${showDetails.time}`);
    console.log(`Venue: ${showDetails.venue}`);
    console.log(`Location: ${showDetails.city}, ${showDetails.state}`);
    console.log(`Price: $${showDetails.price}`);
    console.log(`Age: ${showDetails.age_requirement}`);
    console.log("\nBands:");
    showDetails.bands.forEach((band) => {
      const bandId = band.toLowerCase().replace(/\s+/g, "-");
      const isNew = !bandsData[bandId];
      console.log(`- ${band}${isNew ? " (NEW)" : ""}`);
    });

    // Ask for confirmation
    const { value } = await prompts({
      type: "confirm",
      name: "value",
      message: "Create show with these details?",
      initial: true,
    });

    if (!value) {
      console.log("Cancelled. No files were created.");
      process.exit(0);
    }

    // Convert band names to IDs and update bands.yaml
    const bandIds = showDetails.bands.map((bandName) => {
      const bandId = bandName.toLowerCase().replace(/\s+/g, "-");
      if (!bandsData[bandId]) {
        console.log(`Adding new band: ${bandName}`);
        bandsData[bandId] = {
          name: bandName,
        };
      }
      return bandId;
    });

    // Convert venue name to slug and update venues.yaml if needed
    const venueSlug = showDetails.venue.toLowerCase().replace(/\s+/g, "-");
    if (!venuesData[venueSlug]) {
      console.log(`Adding new venue: ${showDetails.venue}`);
      venuesData[venueSlug] = {
        name: showDetails.venue,
        city: showDetails.city,
        state: showDetails.state,
      };
    }

    // Create show markdown file
    const showContent = `---
title: "${showDetails.date} ${showDetails.bands.join(" ")}"
date: ${new Date().toISOString()}
event_date: ${showDetails.date}T${showDetails.time}:00-07:00
draft: false
venues:
  - "${venueSlug}"
city: "${showDetails.city}"
state: "${showDetails.state}"${
      showDetails.price
        ? `
price: "${showDetails.price}"`
        : ""
    }
age_requirement: "${showDetails.age_requirement}"
bands:
${bandIds.map((id) => `  - "${id}"`).join("\n")}
---
`;

    // Write files
    const showFileName = `${showDetails.date}-${bandIds.join("-")}.md`;
    const showPath = path.join(process.cwd(), "content", "shows", showFileName);

    fs.writeFileSync(showPath, showContent);
    fs.writeFileSync(bandsPath, yaml.dump(bandsData));
    fs.writeFileSync(venuesPath, yaml.dump(venuesData));

    console.log(`\nSuccess!`);
    console.log(`Created show file: ${showFileName}`);
    if (bandIds.some((id) => !bandsData[id])) {
      console.log(
        "\nNew bands added to bands.yaml. You may want to add social links."
      );
    }
    if (!venuesData[venueSlug]) {
      console.log(
        "\nNew venue added to venues.yaml. You may want to add address and social links."
      );
    }
  } catch (error) {
    console.error("Error:", error.message);
  } finally {
    process.exit(0);
  }
}

main();
