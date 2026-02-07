#!/usr/bin/env bun
import { spawn } from 'child_process';
import path from 'path';
import fs from 'fs';
import { fileURLToPath } from 'url';
import { checkbox, confirm, select } from '@inquirer/prompts';
import { previewEvents, scrapeTicketWebVenue, VENUES } from './scrape-ticketweb-venue.js';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const BACKEND_DIR = path.resolve(__dirname, '../../backend');

// Environment configurations
const ENVIRONMENTS = {
  stage: {
    name: 'Stage',
    envFile: '.env.stage',
  },
  production: {
    name: 'Production',
    envFile: '.env.production',
  },
};

/**
 * Run the Go import tool
 */
async function runImport(jsonFile, envFile, dryRun = false) {
  return new Promise((resolve, reject) => {
    const args = [
      'run', './cmd/discovery-import',
      '-input', jsonFile,
      '-env', envFile,
      '-verbose',
    ];
    if (dryRun) args.push('-dry-run');

    console.log(`\n  Running: go ${args.join(' ')}`);

    const proc = spawn('go', args, {
      cwd: BACKEND_DIR,
      stdio: 'inherit',
    });

    proc.on('close', code => {
      if (code === 0) {
        resolve();
      } else {
        reject(new Error(`Import failed with exit code ${code}`));
      }
    });

    proc.on('error', reject);
  });
}

/**
 * Write events to a temporary JSON file
 */
function writeEventsToTemp(events) {
  const tempDir = path.join(__dirname, 'output');
  if (!fs.existsSync(tempDir)) {
    fs.mkdirSync(tempDir, { recursive: true });
  }

  const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
  const filename = `import-${timestamp}.json`;
  const filepath = path.join(tempDir, filename);

  fs.writeFileSync(filepath, JSON.stringify(events, null, 2));
  return filepath;
}

/**
 * Interactive flow for a single venue
 */
async function scrapeVenueInteractive(venueSlug) {
  const venueName = VENUES[venueSlug]?.name || venueSlug;
  console.log(`\n${'='.repeat(60)}`);
  console.log(`Venue: ${venueName}`);
  console.log('='.repeat(60));

  // Quick preview
  const preview = await previewEvents(venueSlug);

  if (preview.length === 0) {
    console.log('No events found.');
    return [];
  }

  // Event selection
  const selected = await checkbox({
    message: `Select events from ${venueName}:`,
    choices: preview.map(e => ({
      name: `${e.date} - ${e.title}`,
      value: e.id,
    })),
    pageSize: 15,
  });

  if (selected.length === 0) {
    console.log('No events selected.');
    return [];
  }

  console.log(`\nScraping ${selected.length} events...`);

  // Full scrape of selected events
  const events = await scrapeTicketWebVenue(venueSlug, { eventIds: selected });

  // Show what was scraped
  console.log(`\nScraped ${events.length} events:`);
  events.forEach((e, i) => {
    console.log(`  ${i + 1}. ${e.title} (${e.date})`);
    console.log(`     Artists: ${e.artists.join(', ')}`);
  });

  return events;
}

/**
 * Main interactive flow
 */
async function main() {
  console.log('Venue Discovery & Importer');
  console.log('===========================\n');

  // Step 1: Select venues
  const venueChoices = Object.entries(VENUES).map(([slug, config]) => ({
    name: config.name,
    value: slug,
  }));

  const selectedVenues = await checkbox({
    message: 'Select venues to scrape:',
    choices: venueChoices,
  });

  if (selectedVenues.length === 0) {
    console.log('No venues selected. Exiting.');
    process.exit(0);
  }

  // Step 2: Scrape each venue interactively
  let allEvents = [];

  for (const venueSlug of selectedVenues) {
    const events = await scrapeVenueInteractive(venueSlug);
    allEvents = allEvents.concat(events);
  }

  if (allEvents.length === 0) {
    console.log('\nNo events to import. Exiting.');
    process.exit(0);
  }

  console.log(`\n${'='.repeat(60)}`);
  console.log(`Total: ${allEvents.length} events ready for import`);
  console.log('='.repeat(60));

  // Step 3: Select target environment(s)
  const targetEnv = await select({
    message: 'Where do you want to import these events?',
    choices: [
      { name: 'Stage only', value: 'stage' },
      { name: 'Production only', value: 'production' },
      { name: 'Stage first, then Production', value: 'both' },
      { name: 'Save JSON only (no import)', value: 'none' },
    ],
  });

  // Write events to JSON file
  const jsonFile = writeEventsToTemp(allEvents);
  console.log(`\nEvents saved to: ${jsonFile}`);

  if (targetEnv === 'none') {
    console.log('\nDone! Use the Go import tool to import later:');
    console.log(`  cd ${BACKEND_DIR}`);
    console.log(`  go run ./cmd/discovery-import -input ${jsonFile} -env .env.staging`);
    process.exit(0);
  }

  // Step 4: Dry run first
  const environments = targetEnv === 'both'
    ? ['stage', 'production']
    : [targetEnv];

  for (const env of environments) {
    const envConfig = ENVIRONMENTS[env];
    console.log(`\n${'='.repeat(60)}`);
    console.log(`DRY RUN: ${envConfig.name}`);
    console.log('='.repeat(60));

    try {
      await runImport(jsonFile, envConfig.envFile, true);
    } catch (err) {
      console.error(`Dry run failed for ${envConfig.name}:`, err.message);
      const continueAnyway = await confirm({
        message: 'Continue anyway?',
        default: false,
      });
      if (!continueAnyway) {
        process.exit(1);
      }
    }

    // Confirm actual import
    const doImport = await confirm({
      message: `Proceed with actual import to ${envConfig.name}?`,
      default: true,
    });

    if (doImport) {
      console.log(`\nImporting to ${envConfig.name}...`);
      try {
        await runImport(jsonFile, envConfig.envFile, false);
        console.log(`\n✓ Successfully imported to ${envConfig.name}`);
      } catch (err) {
        console.error(`Import failed for ${envConfig.name}:`, err.message);

        if (env === 'stage' && environments.includes('production')) {
          const continueToProduction = await confirm({
            message: 'Stage import failed. Continue to Production anyway?',
            default: false,
          });
          if (!continueToProduction) {
            process.exit(1);
          }
        }
      }
    } else {
      console.log(`Skipped ${envConfig.name}`);
    }
  }

  console.log('\n✓ All done!');
  console.log(`JSON file saved at: ${jsonFile}`);
}

// Run
main().catch(err => {
  console.error('Error:', err);
  process.exit(1);
});
