import { chromium } from 'playwright';

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

async function scrapeValleyBar() {
  console.log('Launching browser...');
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  try {
    console.log('Navigating to Valley Bar calendar...');
    await page.goto('https://www.valleybarphx.com/calendar/', {
      waitUntil: 'domcontentloaded',
      timeout: 60000,
    });

    // Wait for the all_events variable to be defined
    console.log('Waiting for calendar data to load...');
    await page.waitForFunction(() => typeof all_events !== 'undefined', {
      timeout: 30000,
    });

    // First, let's see all the fields available in the raw event data
    const rawSample = await page.evaluate(() => {
      if (typeof all_events !== 'undefined' && all_events.length > 0) {
        return { keys: Object.keys(all_events[0]), sample: all_events[0] };
      }
      return null;
    });

    console.log('\nRaw event object keys:', rawSample?.keys);
    console.log('\nSample raw event:');
    console.log(JSON.stringify(rawSample?.sample, null, 2));

    // Extract all events
    const events = await page.evaluate(() => {
      if (typeof all_events !== 'undefined') {
        return all_events;
      }
      return null;
    });

    if (!events) {
      console.log('Could not find all_events array on the page.');
      return [];
    }

    console.log(`\n${'='.repeat(80)}`);
    console.log(`Found ${events.length} raw events`);

    // Also try to get ticket links from the page's event dialogs
    const ticketLinks = await page.evaluate(() => {
      const links = {};
      // Look for ticket links in the page
      document.querySelectorAll('[id^="tw-event-dialog-"] a[href*="ticketweb"]').forEach(a => {
        const dialog = a.closest('[id^="tw-event-dialog-"]');
        if (dialog) {
          const id = dialog.id.replace('tw-event-dialog-', '');
          links[id] = a.href;
        }
      });
      return links;
    });

    console.log(`\nFound ${Object.keys(ticketLinks).length} ticket links`);

    // Process events with cleanup
    const processedEvents = events.map(event => {
      const eventId = event.id;
      return {
        id: eventId,
        title: decodeHtmlEntities(event.title),
        date: event.start,
        venue: stripHtml(event.venue) || 'Valley Bar',
        imageUrl: extractImageUrl(event.imageUrl),
        doorsTime: parseTime(event.doors),
        showTime: parseTime(event.displayTime),
        ticketUrl: ticketLinks[eventId] || null,
      };
    });

    // Display sample of processed events
    console.log('\n' + '='.repeat(80));
    console.log('\nProcessed events sample:\n');
    processedEvents.slice(0, 5).forEach((event, i) => {
      console.log(`${i + 1}. ${event.title}`);
      console.log(`   Date: ${event.date}`);
      console.log(`   Doors: ${event.doorsTime || 'N/A'} | Show: ${event.showTime || 'N/A'}`);
      console.log(`   Tickets: ${event.ticketUrl || 'N/A'}`);
      console.log('');
    });

    // Output clean JSON
    console.log('\n' + '='.repeat(80));
    console.log('\nClean event data (first 5):');
    console.log(JSON.stringify(processedEvents.slice(0, 5), null, 2));

    return processedEvents;
  } catch (error) {
    console.error('Error scraping:', error.message);
    throw error;
  } finally {
    await browser.close();
  }
}

scrapeValleyBar()
  .then(events => {
    console.log(`\n${'='.repeat(80)}`);
    console.log(`Successfully extracted ${events.length} events`);
  })
  .catch(err => {
    console.error('Scraping failed:', err);
    process.exit(1);
  });
