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

async function main() {
  try {
    // Get the current date for the filename and front matter
    const now = new Date();
    const dateString = formatDate(now);

    // Get post details from user
    const response = await prompts([
      {
        type: "text",
        name: "title",
        message: "Enter post title (or press Enter to fill in later):",
        initial: "",
      },
      {
        type: "text",
        name: "description",
        message: "Enter post description (or press Enter to fill in later):",
        initial: "",
      },
      {
        type: "list",
        name: "categories",
        message: "Enter categories (comma-separated, or press Enter for none):",
        initial: "",
        separator: ",",
      },
    ]);

    // Generate the front matter
    const frontMatter = {
      title: response.title || "",
      date: now.toISOString(),
      categories: response.categories.filter((cat) => cat.trim()),
      description: response.description || "",
    };

    // Create the markdown content
    const content = `---
title: "${frontMatter.title}"
date: ${frontMatter.date}
categories: ${JSON.stringify(frontMatter.categories)}
description: "${frontMatter.description}"
---

`;

    // Generate filename using date and title slug
    const slug = response.title ? createSlug(response.title) : "new-post";
    const filename = `${dateString}-${slug}.md`;
    const filePath = path.join(process.cwd(), "content", "blog", filename);

    // Ensure the blog directory exists
    const blogDir = path.join(process.cwd(), "content", "blog");
    if (!fs.existsSync(blogDir)) {
      fs.mkdirSync(blogDir, { recursive: true });
    }

    // Write the file
    fs.writeFileSync(filePath, content);

    console.log("\nSuccess!");
    console.log(`Created blog post: ${filename}`);
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
