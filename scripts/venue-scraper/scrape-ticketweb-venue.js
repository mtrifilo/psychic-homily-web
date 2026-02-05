import { chromium } from 'playwright';
import fs from 'fs';
import path from 'path';

/**
 * Generic scraper for venues using TicketWeb + FullCalendar
 * Works with: Valley Bar, Crescent Ballroom, and likely other Stateside Presents venues
 */

// Venue configurations
export const VENUES = {
  'valley-bar': {
    name: 'Valley Bar',
    url: 'https://www.valleybarphx.com/calendar/',
  },
  'crescent-ballroom': {
    name: 'Crescent Ballroom',
    url: 'https://www.crescentphx.com/calendar/',
  },
};

// Helper to decode HTML entities
function decodeHtmlEntities(text) {
  if (!text) return text;
  return text
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&#8211;/g, '–')
    .replace(/&#8212;/g, '—')
    .replace(/&#8217;/g, "'")
    .replace(/&#8216;/g, "'")
    .replace(/&#8220;/g, '"')
    .replace(/&#8221;/g, '"');
}

// Extract image URL from img tag HTML
function extractImageUrl(imgHtml) {
  if (!imgHtml) return null;
  const match = imgHtml.match(/src="([^"]+)"/);
  return match ? match[1] : null;
}

// Extract text from HTML
function stripHtml(html) {
  if (!html) return null;
  return html.replace(/<[^>]*>/g, '').trim();
}

// Parse time string like "Show: 7:00 pm" or "Doors: 6:30 pm"
function parseTime(timeStr) {
  if (!timeStr) return null;
  const match = timeStr.match(/(\d{1,2}:\d{2}\s*[ap]m)/i);
  return match ? match[1] : null;
}

// Words that should stay lowercase in titles (unless first word)
const LOWERCASE_WORDS = new Set(['a', 'an', 'the', 'and', 'but', 'or', 'for', 'nor', 'on', 'at', 'to', 'by', 'with', 'of', 'in']);

// Words/patterns that should stay uppercase
const UPPERCASE_PATTERNS = [/^(dj|mc|vs\.?|ft\.?|feat\.?)$/i, /^[A-Z]{2,4}$/]; // DJ, MC, VS, FT, state abbreviations

/**
 * Convert text to Title Case
 * "ARMAND HAMMER" -> "Armand Hammer"
 * "DJ SHADOW" -> "DJ Shadow"
 * "THE NEW PORNOGRAPHERS" -> "The New Pornographers"
 * "graham hunt" -> "Graham Hunt"
 * @param {string} str - Input string
 * @param {boolean} force - Force title case even if already mixed case (for artist names)
 */
function toTitleCase(str, force = false) {
  if (!str) return str;

  // If not forcing, skip strings that appear already formatted (mixed case)
  if (!force) {
    const upperCount = (str.match(/[A-Z]/g) || []).length;
    const lowerCount = (str.match(/[a-z]/g) || []).length;
    if (lowerCount > upperCount) return str;
  }

  return str
    .toLowerCase()
    .split(' ')
    .map((word, index) => {
      // Check if word should stay uppercase (DJ, MC, etc.)
      for (const pattern of UPPERCASE_PATTERNS) {
        if (pattern.test(word)) {
          return word.toUpperCase();
        }
      }

      // Keep lowercase words lowercase (except first word)
      if (index > 0 && LOWERCASE_WORDS.has(word)) {
        return word;
      }

      // Capitalize first letter, handle hyphenated words
      return word
        .split('-')
        .map(part => part.charAt(0).toUpperCase() + part.slice(1))
        .join('-');
    })
    .join(' ');
}

/**
 * Quick preview of events - extracts basic info without visiting detail pages
 * @param {string} venueSlug - Key from VENUES config (e.g., 'valley-bar')
 * @returns {Promise<Array>} - Array of preview event objects (id, title, date, venue)
 */
export async function previewEvents(venueSlug) {
  const venue = VENUES[venueSlug];
  if (!venue) {
    throw new Error(`Unknown venue: ${venueSlug}. Available: ${Object.keys(VENUES).join(', ')}`);
  }

  console.log(`\nFetching event list from ${venue.name}...`);

  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  try {
    await page.goto(venue.url, {
      waitUntil: 'domcontentloaded',
      timeout: 60000,
    });

    // Wait for the all_events variable
    await page.waitForFunction(() => typeof all_events !== 'undefined', {
      timeout: 30000,
      polling: 500,
    });

    // Extract basic event info only
    const events = await page.evaluate(() => {
      if (typeof all_events !== 'undefined') {
        return all_events.map(e => ({
          id: e.id,
          title: e.title,
          date: e.start,
          venue: e.venue,
        }));
      }
      return [];
    });

    // Process the titles
    const previewEvents = events.map(e => ({
      id: e.id,
      title: toTitleCase(decodeHtmlEntities(e.title)),
      date: e.date,
      venue: stripHtml(e.venue) || venue.name,
    }));

    console.log(`  ✓ Found ${previewEvents.length} events\n`);
    return previewEvents;
  } finally {
    await browser.close();
  }
}

/**
 * Scrape a TicketWeb-powered venue calendar
 * @param {string} venueSlug - Key from VENUES config (e.g., 'valley-bar')
 * @param {Object} options - Scraping options
 * @param {Array<string>} options.eventIds - Optional list of event IDs to scrape (null = all)
 * @returns {Promise<Array>} - Array of processed event objects
 */
export async function scrapeTicketWebVenue(venueSlug, options = {}) {
  const { eventIds = null } = options;

  const venue = VENUES[venueSlug];
  if (!venue) {
    throw new Error(`Unknown venue: ${venueSlug}. Available: ${Object.keys(VENUES).join(', ')}`);
  }

  console.log(`\nScraping ${venue.name}...`);
  console.log(`URL: ${venue.url}`);

  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  // Add console message listener for debugging
  page.on('console', msg => {
    if (process.env.DEBUG) {
      console.log(`  [browser] ${msg.text()}`);
    }
  });

  try {
    console.log('  Loading page...');
    await page.goto(venue.url, {
      waitUntil: 'domcontentloaded',
      timeout: 60000,
    });
    console.log('  Page loaded, waiting for calendar data...');

    // Wait for the all_events variable to be defined with better error handling
    try {
      await page.waitForFunction(() => typeof all_events !== 'undefined', {
        timeout: 30000,
        polling: 500, // Check every 500ms
      });
    } catch (waitError) {
      // Check what variables are available for debugging
      const availableVars = await page.evaluate(() => {
        const vars = [];
        if (typeof all_events !== 'undefined') vars.push('all_events');
        if (typeof events !== 'undefined') vars.push('events');
        if (typeof calendarEvents !== 'undefined') vars.push('calendarEvents');
        // Check for FullCalendar
        if (typeof FullCalendar !== 'undefined') vars.push('FullCalendar');
        if (document.querySelector('.fc-event')) vars.push('.fc-event elements');
        return vars;
      });
      console.log(`  Available: ${availableVars.length > 0 ? availableVars.join(', ') : 'none found'}`);
      throw new Error(`Timeout waiting for all_events variable (30s). Page may have different structure.`);
    }

    // Extract all events
    console.log('  Extracting events...');
    let events = await page.evaluate(() => {
      if (typeof all_events !== 'undefined') {
        return all_events;
      }
      return null;
    });

    if (!events || events.length === 0) {
      console.log('  No events found.');
      return [];
    }

    console.log(`  Found ${events.length} raw events`);

    // Filter events if eventIds provided
    if (eventIds && eventIds.length > 0) {
      const eventIdSet = new Set(eventIds);
      events = events.filter(e => eventIdSet.has(e.id));
      console.log(`  Filtered to ${events.length} selected events`);
    }

    // Get ticket links from event dialogs
    console.log('  Extracting ticket links...');
    const ticketLinks = await page.evaluate(() => {
      const links = {};
      document.querySelectorAll('[id^="tw-event-dialog-"] a[href*="ticketweb"]').forEach(a => {
        const dialog = a.closest('[id^="tw-event-dialog-"]');
        if (dialog) {
          const id = dialog.id.replace('tw-event-dialog-', '');
          links[id] = a.href;
        }
      });
      return links;
    });

    console.log(`  Found ${Object.keys(ticketLinks).length} ticket links`);

    // Get event detail URLs from dialogs
    console.log('  Extracting event detail URLs...');
    const eventUrls = await page.evaluate(() => {
      const urls = {};
      document.querySelectorAll('[id^="tw-event-dialog-"] .tw-name a').forEach(a => {
        const dialog = a.closest('[id^="tw-event-dialog-"]');
        if (dialog) {
          const id = dialog.id.replace('tw-event-dialog-', '');
          urls[id] = a.href;
        }
      });
      return urls;
    });

    // Process events with artist details from individual pages
    console.log(`  Fetching artist details for ${events.length} events...`);
    const processedEvents = [];

    for (let i = 0; i < events.length; i++) {
      const event = events[i];
      const eventUrl = eventUrls[event.id];

      // Show progress for each event with title (truncate long titles)
      const title = decodeHtmlEntities(event.title);
      const displayTitle = title.length > 40 ? title.slice(0, 37) + '...' : title;
      process.stdout.write(`  [${i + 1}/${events.length}] ${displayTitle}`.padEnd(60) + '\r');

      let artists = [];

      // Fetch artist list from event detail page
      if (eventUrl) {
        try {
          const detailPage = await browser.newPage();
          await detailPage.goto(eventUrl, { waitUntil: 'domcontentloaded', timeout: 15000 });

          artists = await detailPage.evaluate(() => {
            const artistList = [];
            document.querySelectorAll('.artist-list .row h4 a').forEach(a => {
              const name = a.textContent.trim();
              if (name) artistList.push(name);
            });
            return artistList;
          });

          // Apply title case to artist names (force=true to fix inconsistent casing)
          artists = artists.map(name => toTitleCase(name, true));

          await detailPage.close();
        } catch (err) {
          // If we can't get artist details, fall back to title parsing
          artists = [];
        }
      }

      // Fall back to title if no artists found
      if (artists.length === 0) {
        const title = toTitleCase(decodeHtmlEntities(event.title));
        // Remove tour name suffixes like "– Some Tour Name" for artist parsing
        const cleanTitle = title.replace(/\s*[–-]\s*[^–-]*tour.*$/i, '').trim();
        artists = [cleanTitle];
      }

      processedEvents.push({
        id: event.id,
        title: toTitleCase(decodeHtmlEntities(event.title)),
        date: event.start,
        venue: stripHtml(event.venue) || venue.name,
        venueSlug: venueSlug,
        imageUrl: extractImageUrl(event.imageUrl),
        doorsTime: parseTime(event.doors),
        showTime: parseTime(event.displayTime),
        ticketUrl: ticketLinks[event.id] || null,
        artists: artists, // Full artist list from detail page
        scrapedAt: new Date().toISOString(),
      });
    }

    console.log(`\n  ✓ Processed ${processedEvents.length} events`);
    return processedEvents;
  } catch (error) {
    console.error(`  ✗ Error: ${error.message}`);
    throw error;
  } finally {
    await browser.close();
  }
}

/**
 * Scrape all configured TicketWeb venues
 * @returns {Promise<Object>} - Object with venue slugs as keys and event arrays as values
 */
export async function scrapeAllVenues() {
  const results = {};

  for (const venueSlug of Object.keys(VENUES)) {
    try {
      results[venueSlug] = await scrapeTicketWebVenue(venueSlug);
    } catch (error) {
      console.error(`Failed to scrape ${venueSlug}:`, error.message);
      results[venueSlug] = { error: error.message };
    }
  }

  return results;
}

/**
 * Write events to a JSON file
 * @param {Array} events - Array of event objects
 * @param {string} outputDir - Output directory path
 * @returns {string} - Path to the written file
 */
function writeEventsToFile(events, outputDir) {
  // Ensure output directory exists
  if (!fs.existsSync(outputDir)) {
    fs.mkdirSync(outputDir, { recursive: true });
  }

  // Generate filename with timestamp
  const now = new Date();
  const dateStr = now.toISOString().split('T')[0]; // YYYY-MM-DD
  const filename = `scraped-events-${dateStr}.json`;
  const filepath = path.join(outputDir, filename);

  // Write JSON file
  fs.writeFileSync(filepath, JSON.stringify(events, null, 2));
  return filepath;
}

/**
 * Parse command line arguments
 */
function parseArgs() {
  const args = process.argv.slice(2);
  const result = {
    venue: null,
    all: false,
    output: null,
    help: false,
    interactive: false,
  };

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    if (arg === '--all') {
      result.all = true;
    } else if (arg === '--output' || arg === '-o') {
      result.output = args[++i];
    } else if (arg === '--help' || arg === '-h') {
      result.help = true;
    } else if (arg === '--interactive' || arg === '-i') {
      result.interactive = true;
    } else if (!arg.startsWith('-')) {
      result.venue = arg;
    }
  }

  return result;
}

// CLI usage
const options = parseArgs();

if (options.help) {
  console.log('TicketWeb Venue Scraper');
  console.log('=======================\n');
  console.log('Usage:');
  console.log('  node scrape-ticketweb-venue.js <venue-slug>              Scrape a specific venue');
  console.log('  node scrape-ticketweb-venue.js <venue-slug> -i           Interactive mode (select events)');
  console.log('  node scrape-ticketweb-venue.js --all                     Scrape all venues');
  console.log('  node scrape-ticketweb-venue.js --all --output ./output   Scrape and save to directory\n');
  console.log('Options:');
  console.log('  --interactive, -i   Interactive mode: preview events and select which to scrape');
  console.log('  --all               Scrape all configured venues');
  console.log('  --output, -o <dir>  Output directory for JSON file');
  console.log('  --help, -h          Show this help message\n');
  console.log('Available venues:');
  for (const [slug, config] of Object.entries(VENUES)) {
    console.log(`  ${slug.padEnd(20)} ${config.name}`);
  }
  process.exit(0);
}

if (options.all) {
  // Scrape all venues
  scrapeAllVenues()
    .then(results => {
      console.log('\n' + '='.repeat(80));
      console.log('SUMMARY');
      console.log('='.repeat(80));

      // Flatten results for output
      let allEvents = [];
      for (const [venue, events] of Object.entries(results)) {
        if (events.error) {
          console.log(`${venue}: ERROR - ${events.error}`);
        } else {
          console.log(`${venue}: ${events.length} events`);
          allEvents = allEvents.concat(events);
        }
      }

      // Write to file if output directory specified
      if (options.output) {
        const filepath = writeEventsToFile(allEvents, options.output);
        console.log(`\nEvents written to: ${filepath}`);
        console.log(`Total events saved: ${allEvents.length}`);
      }
    })
    .catch(err => {
      console.error('Scraping failed:', err);
      process.exit(1);
    });
} else if (options.interactive && options.venue && VENUES[options.venue]) {
  // Interactive mode: preview then select events
  (async () => {
    try {
      // 1. Quick preview
      const preview = await previewEvents(options.venue);

      if (preview.length === 0) {
        console.log('No events found.');
        process.exit(0);
      }

      // 2. Checkbox selection
      const { checkbox } = await import('@inquirer/prompts');
      const selected = await checkbox({
        message: 'Select events to scrape (space to toggle, enter to confirm):',
        choices: preview.map(e => ({
          name: `${e.date} - ${e.title}`,
          value: e.id,
        })),
        pageSize: 15,
      });

      if (selected.length === 0) {
        console.log('\nNo events selected. Exiting.');
        process.exit(0);
      }

      console.log(`\nSelected ${selected.length} events for full scraping...`);

      // 3. Full scrape of selected only
      const events = await scrapeTicketWebVenue(options.venue, { eventIds: selected });

      // 4. Output results
      console.log('\n' + '='.repeat(80));
      console.log(`\nScraped events from ${VENUES[options.venue].name}:\n`);
      events.forEach((event, i) => {
        console.log(`${i + 1}. ${event.title}`);
        console.log(`   Date: ${event.date} | Doors: ${event.doorsTime || 'N/A'} | Show: ${event.showTime || 'N/A'}`);
        console.log(`   Artists: ${event.artists.join(', ')}`);
        console.log(`   Tickets: ${event.ticketUrl || 'N/A'}`);
        console.log('');
      });
      console.log(`Total: ${events.length} events`);

      // Write to file if output directory specified
      if (options.output) {
        const filepath = writeEventsToFile(events, options.output);
        console.log(`\nEvents written to: ${filepath}`);
      }
    } catch (err) {
      console.error('Scraping failed:', err);
      process.exit(1);
    }
  })();
} else if (options.venue && VENUES[options.venue]) {
  // Scrape specific venue (all events)
  scrapeTicketWebVenue(options.venue)
    .then(events => {
      console.log('\n' + '='.repeat(80));
      console.log(`\nSample events from ${VENUES[options.venue].name}:\n`);
      events.slice(0, 5).forEach((event, i) => {
        console.log(`${i + 1}. ${event.title}`);
        console.log(`   Date: ${event.date} | Doors: ${event.doorsTime || 'N/A'} | Show: ${event.showTime || 'N/A'}`);
        console.log(`   Tickets: ${event.ticketUrl || 'N/A'}`);
        console.log('');
      });
      console.log(`Total: ${events.length} events`);

      // Write to file if output directory specified
      if (options.output) {
        const filepath = writeEventsToFile(events, options.output);
        console.log(`\nEvents written to: ${filepath}`);
      }
    })
    .catch(err => {
      console.error('Scraping failed:', err);
      process.exit(1);
    });
} else {
  console.log('TicketWeb Venue Scraper');
  console.log('=======================\n');
  console.log('Usage:');
  console.log('  node scrape-ticketweb-venue.js <venue-slug>              Scrape a specific venue');
  console.log('  node scrape-ticketweb-venue.js <venue-slug> -i           Interactive mode (select events)');
  console.log('  node scrape-ticketweb-venue.js --all                     Scrape all venues');
  console.log('  node scrape-ticketweb-venue.js --all --output ./output   Scrape and save to directory\n');
  console.log('Available venues:');
  for (const [slug, config] of Object.entries(VENUES)) {
    console.log(`  ${slug.padEnd(20)} ${config.name}`);
  }
}
