#!/usr/bin/env node

const fs = require("fs");
const path = require("path");
const prompts = require("prompts");

// Suppress only punycode deprecation warning
process.removeAllListeners("warning");

function formatDate(date) {
  return date.toISOString().split("T")[0];
}

function createSlug(title) {
  return title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/(^-|-$)/g, "");
}

function parseSoundcloudEmbed(embedCode) {
  try {
    // Clean up the input text first
    const cleanedCode = embedCode
      .replace(/\n/g, "")
      .replace(/\s+/g, " ")
      .trim();

    // Extract the player URL
    const playerUrlMatch = cleanedCode.match(/src="([^"]+)"/);
    if (!playerUrlMatch) throw new Error("Could not find player URL");

    // Extract track info - look for the last title attribute which contains the track name
    const titleMatches = [...cleanedCode.matchAll(/title="([^"]+)"/g)];
    const trackTitle = titleMatches[titleMatches.length - 1][1]
      .replace(/&quot;/g, '"') // First convert any existing HTML entities to quotes
      .replace(/"/g, "&quot;"); // Then convert all quotes to HTML entities

    // Extract artist info - look for the first title attribute after soundcloud.com
    const artistMatch = cleanedCode.match(
      /soundcloud\.com\/[^"]+"\s+title="([^"]+)"/
    );
    if (!artistMatch) throw new Error("Could not find artist info");

    // Extract URLs
    const artistUrl = cleanedCode.match(
      /href="(https:\/\/soundcloud\.com\/[^"]+)"/
    )[1];
    const trackUrl = cleanedCode.match(
      /href="(https:\/\/soundcloud\.com\/[^"]+\/[^"]+)"/
    )[1];

    return {
      embedUrl: playerUrlMatch[1],
      title: trackTitle,
      artist: artistMatch[1],
      artistUrl: artistUrl,
      trackUrl: trackUrl,
    };
  } catch (error) {
    console.error("Parsing error:", error.message);
    throw new Error(
      "Failed to parse Soundcloud embed code. Please check your input."
    );
  }
}

function escapeYamlString(str) {
  // If the string contains double quotes, wrap it in single quotes
  if (str.includes('"')) {
    return `'${str}'`;
  }
  // Otherwise wrap in double quotes
  return `"${str}"`;
}

async function main() {
  try {
    const now = new Date();
    const dateString = formatDate(now);

    console.log(
      "Please copy the ENTIRE embed code (both iframe and div) and paste it as a single line."
    );
    const { embedCode } = await prompts({
      type: "text",
      name: "embedCode",
      message: "Paste the Soundcloud embed code:",
      validate: (value) => {
        if (!value.includes("<iframe") || !value.includes("</div>")) {
          return "Please paste the complete embed code (both iframe and div parts)";
        }
        return true;
      },
    });

    if (!embedCode) {
      console.log("No embed code provided. Exiting.");
      process.exit(0);
    }

    // Clean up the input text
    const cleanedCode = embedCode
      .replace(/\n/g, "")
      .replace(/\s+/g, " ")
      .trim();

    console.log("\nParsing embed code...");

    // Extract the player URL
    const playerUrlMatch = cleanedCode.match(/src="([^"]+)"/);
    if (!playerUrlMatch) throw new Error("Could not find player URL");

    // Extract track info - look for the last title attribute which contains the track name
    const titleMatches = [...cleanedCode.matchAll(/title="([^"]+)"/g)];
    const trackTitle = titleMatches[titleMatches.length - 1][1].replace(
      /&quot;/g,
      '"'
    );

    // Extract artist info - look for the first title attribute after soundcloud.com
    const artistMatch = cleanedCode.match(
      /soundcloud\.com\/[^"]+"\s+title="([^"]+)"/
    );
    if (!artistMatch) throw new Error("Could not find artist info");

    // Extract URLs
    const artistUrl = cleanedCode.match(
      /href="(https:\/\/soundcloud\.com\/[^"]+)"/
    )[1];
    const trackUrl = cleanedCode.match(
      /href="(https:\/\/soundcloud\.com\/[^"]+\/[^"]+)"/
    )[1];

    const parsed = {
      embedUrl: playerUrlMatch[1],
      title: trackTitle,
      artist: artistMatch[1],
      artistUrl: artistUrl,
      trackUrl: trackUrl,
    };

    console.log("\nParsed data:", parsed);

    const { description } = await prompts({
      type: "text",
      name: "description",
      message: "Enter mix description:",
      validate: (value) =>
        value.length > 0 ? true : "Description is required",
    });

    const content = `---
title: ${escapeYamlString(parsed.title)}
date: "${now.toISOString()}"
description: ${escapeYamlString(description)}
artist: ${escapeYamlString(parsed.artist)}
soundcloud_url: "${parsed.embedUrl}"
artist_url: "${parsed.artistUrl}"
track_url: "${parsed.trackUrl}"
---

${description}

{{< soundcloud url="${parsed.embedUrl}" title="${parsed.title.replace(
      /"/g,
      "&quot;"
    )}" artist="${parsed.artist}" artist_url="${parsed.artistUrl}" track_url="${
      parsed.trackUrl
    }" >}}`;

    const slug = createSlug(parsed.title);
    const filename = `${dateString}-${slug}.md`;
    const filePath = path.join(process.cwd(), "content", "mixes", filename);

    const mixesDir = path.join(process.cwd(), "content", "mixes");
    if (!fs.existsSync(mixesDir)) {
      fs.mkdirSync(mixesDir, { recursive: true });
    }

    fs.writeFileSync(filePath, content);

    console.log("\nSuccess!");
    console.log(`Created mix post: ${filename}`);
    console.log("\nFile contents preview:");
    console.log("-------------------");
    console.log(content);
  } catch (error) {
    console.error("Error:", error.message);
  } finally {
    process.exit(0);
  }
}

main();
