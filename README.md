# Psychic Homily Web

### [https://psychichomily.com](https://psychichomily.com)

A Hugo-based website to document and amplify some of the most exciting and memorable new music releases, shows, and cultural events from Arizona musicians and beyond, focusing on artists truly putting their hearts and souls into their work bravely.

This website features a show list and blog, along with helper scripts for parsing data. An Anthropic API key is required for the show parser, which uses Claude's Sonnet 3.5 model for an agentic AI workflow to parse natural language show descriptions into structured data for new show listings.

<img width="640" alt="Screenshot 2025-02-16 at 3 11 09 AM" src="https://github.com/user-attachments/assets/072b7211-05a4-45e4-9243-0187d37b2aef" />

## Prerequisites

- [Hugo](https://gohugo.io/installation/)
- Node.js and pnpm
- An Anthropic API key (for the show parser)

## Setup

1. Clone the repository:

```bash
git clone https://github.com/mtrifilo/psychic-homily-web.git
cd psychic-homily-web
```

2. Install Node.js dependencies:

```bash
pnpm install
```

3. Create a `.env` file in the root directory with your Anthropic API key:

```
ANTHROPIC_API_KEY=your_api_key_here
```

## Running the Development Servers

To work on the site locally, you can use one of the following commands. Note that this project uses `pnpm`.

- **`pnpm dev`**: Starts development servers for both the React components and the Hugo site. The Hugo site is available at `http://localhost:1313` with live reload. This is the primary command to use for most development work.

- **`pnpm dev:components`**: Runs a development server for only the React components at `http://localhost:5173`. This is useful for working on components in isolation with fast hot-reloading.

## Adding New Shows

There are two ways to add new shows:

### 1. Using the Show Parser Script

The project includes a custom script that uses Claude AI to parse natural language show announcements into structured data.

https://github.com/user-attachments/assets/0612af17-5d4a-4594-92c3-d8712ccba77f

1. Run the parser:

```bash
pnpm new-show
```

2. Enter the show announcement in the following format:

```
Band Name 1 and Band Name 2
Friday March 15th at Venue Name
8pm • 21+ • $10
```

3. Review the parsed details and confirm to create the show.

The script will:

- Parse the show details
- Add any new bands to `data/bands.yaml`
- Create a new show markdown file in `content/shows/`

### 2. Manual Creation

You can also manually create show files:

1. Create a new markdown file in `content/shows/` with the format `YYYY-MM-DD-band-names.md`
2. Add the required front matter:

```yaml
---
title: "YYYY-MM-DD Show"
date: <current-datetime>
event_date: YYYY-MM-DDT20:00:00-07:00
draft: false
venue: "Venue Name"
city: "City"
state: "ST"
price: "10"
age_requirement: "21+"
bands:
  - "band-id-1"
  - "band-id-2"
---
```

## Adding Bandcamp Embeds

The site includes a custom Bandcamp embed parser and shortcode for easily adding music players to your content.

### Using the Bandcamp Parser

1. Copy the embed code from Bandcamp (Share/Embed → Embed Code)

2. Run the parser:

```bash
pnpm parse-bandcamp
```

3. Paste the embed code when prompted
4. Review the parsed details
5. Confirm to copy the generated Hugo shortcode to your clipboard

### Using the Bandcamp Shortcode

Add Bandcamp players to your content using the shortcode:

```
{{< bandcamp
    album="albumID"
    artist="artist-name"
    title="album-title"
>}}
```

also note that an albumID is all that's needed at a minimum:

```
{{< bandcamp
    album="albumID"
>}}
```

for tracks:

```
{{< bandcamp
    track="trackID"
    artist="artist-name"
    title="track-title"
>}}
```

Optional parameters:

- `size`: "large" (default) or "small"
- `artwork`: "small" (default) or "large"
- `height`: height in pixels (default: 120)
- `bgcol`: background color hex code (default: ffffff)
- `linkcol`: link color hex code (default: 0687f5)
- `tracklist`: "false" (default) or "true"

## Adding Blog Posts

Blog posts can be added to the `content/blog/` directory. While you can create them manually, it is recommended to use the helper scripts:

```bash
pnpm new-blog
```

This script will create a new blog post file for you in `content/blog/`.

There is also a script for creating "mix" posts:

```bash
pnpm new-mix
```

### Blog Post Front Matter

Each blog post should include the following front matter:

```yaml
---
title: "Your Blog Post Title"
date: YYYY-MM-DDT00:00:00-07:00
draft: true
description: "A brief description of your post"
featured_image: "/images/your-image.jpg" # Optional
tags: ["tag1", "tag2"] # Optional
---
```

### Writing Content

The post content is written in Markdown below the front matter. You can:

- Use standard Markdown syntax
- Include images: `![Alt text](/images/image.jpg)`
- Add Bandcamp embeds using the shortcode (see Bandcamp Embeds section)
- Create links to shows: `[Show link](/shows/YYYY-MM-DD-show-name)`

### Publishing

1. Write your post with `draft: true` while working on it
2. Preview it locally using `hugo server -D`
3. When ready to publish, set `draft: false`
4. The post will appear on the blog page and in the RSS feed

## Building React Components

The project includes React components (located in the `components/` directory) that are integrated into Hugo pages. These components need to be built before the Hugo site can use them.

### Development

For development, you can work on React components with hot-reloading:

```bash
pnpm dev:components
```

This starts a development server for the React components at `http://localhost:5173`.

### Building Components for Production

To build the React components for integration with Hugo:

```bash
cd components
pnpm build
```

This will:

- Compile the React components using Vite
- Output the built files to the `static/js/` directory where Hugo can serve them
- Generate optimized bundles for production use

The built components can then be loaded by Hugo pages (like the Submit page) using script tags that reference the generated bundle files.

### Component Integration

React components are integrated into Hugo pages through custom layouts. For example, the Submit page (`layouts/submit/list.html`) includes:

```html
<!-- React mounting point -->
<div id="root"></div>

<!-- Load React bundle -->
<script type="module" src="/js/submit-form.js"></script>
```

The React component (defined in `components/src/main.tsx`) mounts to the `#root` element, allowing for interactive forms and functionality within the static Hugo site.

## Building for Production

To build the complete site for production deployment:

```bash
pnpm build:prod
```

This command will:

- Build the React components and place them in the correct location for Hugo
- Generate a production-ready site in the `public/` directory
- Minify HTML, CSS, JS, JSON, SVG and XML files
- Remove drafts and future-dated content
- Apply all production optimizations

The generated `public/` directory can then be deployed to any static hosting service like Netlify, Vercel, or GitHub Pages.

### Deployment Checklist

Before deploying:

1. Set `draft: false` for all content you want to publish
2. Update `baseURL` in `config.toml` to match your production domain
3. Ensure all images and assets are optimized
4. Test the built site locally:
   ```bash
   hugo server --minify --environment production
   ```

## Project Structure

- `content/shows/` - Show markdown files
- `data/bands.yaml` - Band information and metadata
- `layouts/` - Hugo templates and layouts
- `scripts/` - Utility scripts including the show parser
- `themes/` - Hugo themes
- `assets/` - Static assets like images and CSS

## Development

- The site uses Hugo for static site generation
- Show data is stored in markdown files with YAML front matter
- Band information is centralized in `data/bands.yaml`
- The show parser script (`scripts/parse-show.js`) uses Claude AI to parse natural language show announcements

## Roadmap

- Store the bands and venue data in a database, and re-generate the yaml data based on persisted data
- Allow users to submit shows, tag "favorite" shows, and create a profile
- Create a mobile app for iPhone to allow for viewing show and band data, along with submitting shows on the go
- Create a flyer image parser to extract show details from flyers, to update the show list
- Social features?

## License

MIT License

Copyright (c) 2025 Psychic Homily Web

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

### Third-Party Licenses

This project uses the following open source components:

- [Hugo Ananke Theme](https://github.com/theNewDynamic/gohugo-theme-ananke) - MIT License, Copyright (c) 2016-2025 Bud Parr
- Font Awesome Icons (via Ananke theme) - Licensed under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/)
