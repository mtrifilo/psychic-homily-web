#!/usr/bin/env node

const prompts = require("prompts");
const { exec } = require("child_process");

// Suppress only punycode deprecation warning
process.removeAllListeners("warning");

async function parseBandcampEmbed(embedHtml) {
  try {
    // Extract album/track ID
    const idMatch = embedHtml.match(/(?:album|track)=(\d+)/);
    const type = embedHtml.includes("album=") ? "album" : "track";

    // Extract artist and title from the anchor tag
    const linkMatch = embedHtml.match(
      /href="https:\/\/([^.]+)\.bandcamp\.com\/(?:album|track)\/([^"]+)"/
    );

    if (!idMatch || !linkMatch) {
      throw new Error(
        "Could not parse Bandcamp embed code. Please ensure you copied the full embed code from Bandcamp."
      );
    }

    return {
      type,
      id: idMatch[1],
      artist: linkMatch[1],
      title: linkMatch[2],
    };
  } catch (error) {
    console.error("\nParsing Error Details:");
    console.error("- Message:", error.message);
    throw new Error(
      "Failed to parse Bandcamp embed. Please check your input and try again."
    );
  }
}

async function copyToClipboard(text) {
  return new Promise((resolve, reject) => {
    const clipboardCmd = process.platform === "darwin" ? "pbcopy" : "clip";
    const child = exec(clipboardCmd, (error) => {
      if (error) {
        reject(error);
        return;
      }
      resolve();
    });
    child.stdin.write(text);
    child.stdin.end();
  });
}

async function main() {
  try {
    const { embedCode } = await prompts({
      type: "text",
      name: "embedCode",
      message:
        "Paste your Bandcamp embed code below (press Enter twice when done):",
      multiline: true,
    });

    if (!embedCode) {
      console.log("No input provided. Exiting.");
      process.exit(0);
    }

    // Clean up the input text
    const cleanedCode = embedCode
      .split(/\n/)
      .map((line) => line.trim())
      .filter(Boolean)
      .join(" ")
      .replace(/\s+/g, " ")
      .trim();

    console.log("\nWorking on input:", cleanedCode);

    // Parse the embed code
    const parsed = await parseBandcampEmbed(cleanedCode);

    // Generate Hugo shortcode
    const shortcode = `{{< bandcamp 
    ${parsed.type}="${parsed.id}"
    artist="${parsed.artist}"
    title="${parsed.title}">}}`;

    // Preview the parsed data
    console.log("\nParsed Bandcamp details:");
    console.log("------------------------");
    console.log(`Type: ${parsed.type}`);
    console.log(`ID: ${parsed.id}`);
    console.log(`Artist: ${parsed.artist}`);
    console.log(`Title: ${parsed.title}`);

    console.log("\nGenerated Hugo shortcode:");
    console.log("------------------------");
    console.log(shortcode);

    // Ask for confirmation to copy to clipboard
    const { value } = await prompts({
      type: "confirm",
      name: "value",
      message: "Copy shortcode to clipboard?",
      initial: true,
    });

    if (value) {
      try {
        await copyToClipboard(shortcode);
        console.log("Shortcode copied to clipboard!");
      } catch (error) {
        console.error("Failed to copy to clipboard:", error.message);
      }
    }
  } catch (error) {
    console.error("Error:", error.message);
  } finally {
    process.exit(0);
  }
}

main();
