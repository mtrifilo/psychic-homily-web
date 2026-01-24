#!/usr/bin/env bun
/**
 * CLI tool for creating new blog posts
 *
 * Usage: bun run blog:new
 */

import { existsSync, readdirSync, readFileSync, writeFileSync } from 'fs'
import { join } from 'path'
import { createInterface } from 'readline'

const BLOG_DIR = join(import.meta.dir, '..', 'content', 'blog')
const EDITOR = process.env.EDITOR || 'code' // Default to VS Code

// ANSI colors
const colors = {
  reset: '\x1b[0m',
  bright: '\x1b[1m',
  dim: '\x1b[2m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  cyan: '\x1b[36m',
}

function log(message: string, color = colors.reset) {
  console.log(`${color}${message}${colors.reset}`)
}

function logStep(step: number, message: string) {
  log(`\n${colors.cyan}[${step}/4]${colors.reset} ${message}`)
}

/**
 * Create a readline interface for prompts
 */
function createPrompt() {
  return createInterface({
    input: process.stdin,
    output: process.stdout,
  })
}

/**
 * Prompt user for input
 */
async function prompt(question: string, defaultValue?: string): Promise<string> {
  const rl = createPrompt()
  const defaultHint = defaultValue ? ` ${colors.dim}(${defaultValue})${colors.reset}` : ''

  return new Promise((resolve) => {
    rl.question(`${question}${defaultHint}: `, (answer) => {
      rl.close()
      resolve(answer.trim() || defaultValue || '')
    })
  })
}

/**
 * Prompt user to select from options or enter custom value
 */
async function promptWithOptions(
  question: string,
  options: string[],
  allowMultiple = false
): Promise<string[]> {
  console.log(`\n${question}`)

  if (options.length > 0) {
    log('\nExisting categories:', colors.dim)
    options.forEach((opt, i) => {
      log(`  ${colors.yellow}${i + 1}${colors.reset}. ${opt}`)
    })
    log(`  ${colors.yellow}0${colors.reset}. Enter new category`, colors.dim)
  }

  const hint = allowMultiple ? 'Enter numbers separated by commas, or 0 for new' : 'Enter number or 0 for new'
  const answer = await prompt(`\n${hint}`)

  if (!answer || answer === '0') {
    const custom = await prompt('Enter category name(s), comma-separated')
    return custom.split(',').map((s) => s.trim()).filter(Boolean)
  }

  const indices = answer.split(',').map((s) => parseInt(s.trim(), 10) - 1)
  const selected = indices
    .filter((i) => i >= 0 && i < options.length)
    .map((i) => options[i])

  if (selected.length === 0) {
    log('No valid selection, skipping categories.', colors.yellow)
    return []
  }

  return selected
}

/**
 * Get existing categories from blog posts
 */
function getExistingCategories(): string[] {
  const categories = new Set<string>()

  if (!existsSync(BLOG_DIR)) {
    return []
  }

  const files = readdirSync(BLOG_DIR).filter(
    (f) => f.endsWith('.md') && !f.startsWith('_')
  )

  for (const file of files) {
    const content = readFileSync(join(BLOG_DIR, file), 'utf-8')
    const match = content.match(/categories:\s*\[(.*?)\]/s)
    if (match) {
      const cats = match[1]
        .split(',')
        .map((s) => s.trim().replace(/["']/g, ''))
        .filter(Boolean)
      cats.forEach((c) => categories.add(c))
    }
  }

  return Array.from(categories).sort()
}

/**
 * Convert title to URL-friendly slug
 */
function slugify(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .substring(0, 60) // Limit length
}

/**
 * Generate filename with date prefix
 */
function generateFilename(title: string): string {
  const date = new Date().toISOString().split('T')[0]
  const slug = slugify(title)
  return `${date}-${slug}.md`
}

/**
 * Generate frontmatter YAML
 */
function generateFrontmatter(
  title: string,
  categories: string[],
  description: string
): string {
  const date = new Date().toISOString()
  const categoriesYaml =
    categories.length > 0
      ? `categories: [${categories.map((c) => `"${c}"`).join(', ')}]`
      : ''

  return `---
title: "${title}"
date: ${date}
${categoriesYaml}
description: "${description}"
---

`
}

/**
 * Generate starter content based on category
 */
function generateStarterContent(categories: string[]): string {
  const isNewRelease = categories.some((c) =>
    c.toLowerCase().includes('release')
  )

  if (isNewRelease) {
    return `<!-- Bandcamp embed: Replace with actual album/track details -->
<!-- <Bandcamp album="ALBUM_ID" artist="ARTIST_SLUG" title="ALBUM_SLUG" /> -->

Write your review here...

`
  }

  return `Write your post here...

`
}

/**
 * Open file in editor
 */
async function openInEditor(filePath: string): Promise<void> {
  const proc = Bun.spawn([EDITOR, filePath], {
    stdout: 'inherit',
    stderr: 'inherit',
  })
  // Don't wait for editor to close
}

/**
 * Main CLI flow
 */
async function main() {
  console.clear()
  log('╔════════════════════════════════════════╗', colors.cyan)
  log('║     📝 New Blog Post Generator         ║', colors.cyan)
  log('╚════════════════════════════════════════╝', colors.cyan)

  // Step 1: Title
  logStep(1, 'What\'s the title of your post?')
  const title = await prompt('Title')

  if (!title) {
    log('\n❌ Title is required. Exiting.', colors.yellow)
    process.exit(1)
  }

  // Step 2: Categories
  logStep(2, 'Select categories for your post')
  const existingCategories = getExistingCategories()
  const categories = await promptWithOptions(
    'Choose from existing or create new:',
    existingCategories,
    true
  )

  // Step 3: Description
  logStep(3, 'Add a short description (for SEO & previews)')
  const description = await prompt('Description', '')

  // Step 4: Generate and save
  logStep(4, 'Creating your blog post...')

  const filename = generateFilename(title)
  const filePath = join(BLOG_DIR, filename)

  // Check if file already exists
  if (existsSync(filePath)) {
    log(`\n⚠️  File already exists: ${filename}`, colors.yellow)
    const overwrite = await prompt('Overwrite? (y/N)')
    if (overwrite.toLowerCase() !== 'y') {
      log('Exiting without changes.', colors.dim)
      process.exit(0)
    }
  }

  const frontmatter = generateFrontmatter(title, categories, description)
  const starterContent = generateStarterContent(categories)
  const content = frontmatter + starterContent

  writeFileSync(filePath, content, 'utf-8')

  // Success message
  console.log('\n')
  log('════════════════════════════════════════', colors.green)
  log('  ✅ Blog post created successfully!', colors.green)
  log('════════════════════════════════════════', colors.green)
  console.log('')
  log(`  📄 File: ${colors.bright}${filename}${colors.reset}`)
  log(`  📁 Path: ${colors.dim}${filePath}${colors.reset}`)
  if (categories.length > 0) {
    log(`  🏷️  Categories: ${categories.join(', ')}`)
  }
  console.log('')

  // Offer to open in editor
  const openEditor = await prompt(`Open in ${EDITOR}? (Y/n)`, 'y')
  if (openEditor.toLowerCase() !== 'n') {
    log(`\nOpening in ${EDITOR}...`, colors.dim)
    await openInEditor(filePath)
  }

  log('\nHappy writing! 🎸\n', colors.cyan)
}

// Run
main().catch((err) => {
  console.error('Error:', err)
  process.exit(1)
})
