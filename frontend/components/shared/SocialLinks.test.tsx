import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SocialLinks } from './SocialLinks'

// All asserts on this primitive use `getByRole('link', { name })` over
// `getByTitle()`, because the `sr-only` <span> is what assistive tech
// actually reads — `title` is only a hover tooltip and isn't guaranteed to
// be the accessible name. Locking in the accessible-name behaviour catches
// a regression where the sr-only span is dropped (a real risk when
// icon-only links get refactored).

describe('SocialLinks', () => {
  describe('null / empty branches', () => {
    it('returns null when social is null', () => {
      const { container } = render(<SocialLinks social={null} />)
      expect(container.firstChild).toBeNull()
    })

    it('returns null when social is undefined', () => {
      const { container } = render(<SocialLinks />)
      expect(container.firstChild).toBeNull()
    })

    it('returns null when all social fields are null', () => {
      const { container } = render(
        <SocialLinks
          social={{
            website: null,
            instagram: null,
            facebook: null,
            twitter: null,
            youtube: null,
            spotify: null,
            bandcamp: null,
            soundcloud: null,
          }}
        />
      )
      expect(container.firstChild).toBeNull()
    })

    it('returns null when all social fields are empty strings', () => {
      const { container } = render(
        <SocialLinks
          social={{
            website: '',
            instagram: '',
            facebook: '',
            twitter: '',
            youtube: '',
            spotify: '',
            bandcamp: '',
            soundcloud: '',
          }}
        />
      )
      // Filter rejects falsy values, including empty strings.
      expect(container.firstChild).toBeNull()
    })
  })

  describe('per-platform href + accessible name', () => {
    it('renders a Website link with full URL passthrough and accessible name', () => {
      render(<SocialLinks social={{ website: 'https://example.com' }} />)
      const link = screen.getByRole('link', { name: 'Website' })
      expect(link).toHaveAttribute('href', 'https://example.com')
      expect(link).toHaveAttribute('target', '_blank')
      expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    })

    it('renders Instagram link from handle with accessible name', () => {
      render(<SocialLinks social={{ instagram: 'bandname' }} />)
      const link = screen.getByRole('link', { name: 'Instagram' })
      expect(link).toHaveAttribute('href', 'https://instagram.com/bandname')
    })

    it('renders Instagram link from handle with @ prefix (strips @)', () => {
      render(<SocialLinks social={{ instagram: '@bandname' }} />)
      const link = screen.getByRole('link', { name: 'Instagram' })
      expect(link).toHaveAttribute('href', 'https://instagram.com/bandname')
    })

    it('passes through a full Instagram URL untouched', () => {
      render(
        <SocialLinks social={{ instagram: 'https://instagram.com/bandname' }} />
      )
      const link = screen.getByRole('link', { name: 'Instagram' })
      expect(link).toHaveAttribute('href', 'https://instagram.com/bandname')
    })

    it('renders Facebook link from handle with accessible name', () => {
      render(<SocialLinks social={{ facebook: 'bandpage' }} />)
      const link = screen.getByRole('link', { name: 'Facebook' })
      expect(link).toHaveAttribute('href', 'https://facebook.com/bandpage')
    })

    it('renders Twitter link with accessible name "Twitter/X"', () => {
      render(<SocialLinks social={{ twitter: 'bandname' }} />)
      // The label is intentionally "Twitter/X" so screen readers announce
      // both names (post-rebrand). Locking the string here catches a
      // regression that drops the X suffix.
      const link = screen.getByRole('link', { name: 'Twitter/X' })
      expect(link).toHaveAttribute('href', 'https://twitter.com/bandname')
    })

    it('renders YouTube link from handle with accessible name', () => {
      render(<SocialLinks social={{ youtube: 'channel123' }} />)
      const link = screen.getByRole('link', { name: 'YouTube' })
      expect(link).toHaveAttribute('href', 'https://youtube.com/channel123')
    })

    it('renders Spotify link from artist path with accessible name', () => {
      render(<SocialLinks social={{ spotify: 'artist/123' }} />)
      const link = screen.getByRole('link', { name: 'Spotify' })
      expect(link).toHaveAttribute('href', 'https://open.spotify.com/artist/123')
    })

    it('renders Bandcamp link from full URL with accessible name', () => {
      render(<SocialLinks social={{ bandcamp: 'https://band.bandcamp.com' }} />)
      const link = screen.getByRole('link', { name: 'Bandcamp' })
      expect(link).toHaveAttribute('href', 'https://band.bandcamp.com')
    })

    it('renders SoundCloud link from handle with accessible name', () => {
      render(<SocialLinks social={{ soundcloud: 'bandname' }} />)
      const link = screen.getByRole('link', { name: 'SoundCloud' })
      expect(link).toHaveAttribute('href', 'https://soundcloud.com/bandname')
    })
  })

  describe('omit-empty branches', () => {
    it('only renders links for non-null social fields', () => {
      render(
        <SocialLinks
          social={{
            instagram: 'band',
            facebook: null,
            twitter: null,
          }}
        />
      )
      expect(screen.getByRole('link', { name: 'Instagram' })).toBeInTheDocument()
      expect(screen.queryByRole('link', { name: 'Facebook' })).not.toBeInTheDocument()
      expect(screen.queryByRole('link', { name: 'Twitter/X' })).not.toBeInTheDocument()
    })

    it('only renders links for non-empty-string social fields', () => {
      render(
        <SocialLinks
          social={{
            instagram: 'band',
            facebook: '',
            twitter: '',
          }}
        />
      )
      // Empty-string fields are filtered out the same way nulls are — both
      // are falsy under the `socialLinks.filter(...)` gate.
      expect(screen.getByRole('link', { name: 'Instagram' })).toBeInTheDocument()
      expect(screen.queryByRole('link', { name: 'Facebook' })).not.toBeInTheDocument()
      expect(screen.queryByRole('link', { name: 'Twitter/X' })).not.toBeInTheDocument()
    })

    it('only renders the exact set of platforms supplied (no stale platforms)', () => {
      render(
        <SocialLinks
          social={{
            instagram: 'band',
            spotify: 'https://open.spotify.com/artist/abc',
          }}
        />
      )
      const links = screen.getAllByRole('link')
      // Two platforms supplied → exactly two links rendered. This catches a
      // regression where the filter pulls in extras.
      expect(links).toHaveLength(2)
      const names = links.map((link) =>
        link.getAttribute('aria-label') ??
        link.querySelector('.sr-only')?.textContent ??
        link.textContent
      )
      expect(names).toEqual(expect.arrayContaining(['Instagram', 'Spotify']))
    })
  })

  describe('multi-platform rendering', () => {
    it('renders multiple social links when multiple are present', () => {
      render(
        <SocialLinks
          social={{
            instagram: 'band',
            spotify: 'https://open.spotify.com/artist/abc',
            website: 'https://band.com',
          }}
        />
      )
      expect(screen.getByRole('link', { name: 'Instagram' })).toBeInTheDocument()
      expect(screen.getByRole('link', { name: 'Spotify' })).toBeInTheDocument()
      expect(screen.getByRole('link', { name: 'Website' })).toBeInTheDocument()
    })

    it('preserves the source-of-truth platform order (website → instagram → facebook → twitter → youtube → spotify → bandcamp → soundcloud)', () => {
      // Reordering the social platforms is a visual decision — if the
      // declared order changes, this test should fail and force a
      // conscious update rather than silently shifting.
      render(
        <SocialLinks
          social={{
            soundcloud: 'sc',
            website: 'https://w.com',
            instagram: 'ig',
            facebook: 'fb',
            twitter: 'tw',
            youtube: 'yt',
            spotify: 'sp',
            bandcamp: 'https://b.bandcamp.com',
          }}
        />
      )
      const links = screen.getAllByRole('link')
      const accessibleNames = links.map(
        (link) => link.querySelector('.sr-only')?.textContent
      )
      expect(accessibleNames).toEqual([
        'Website',
        'Instagram',
        'Facebook',
        'Twitter/X',
        'YouTube',
        'Spotify',
        'Bandcamp',
        'SoundCloud',
      ])
    })

    it('opens every link in a new tab with safe rel', () => {
      render(
        <SocialLinks
          social={{
            instagram: 'a',
            facebook: 'b',
            spotify: 'https://open.spotify.com/c',
          }}
        />
      )
      for (const link of screen.getAllByRole('link')) {
        expect(link).toHaveAttribute('target', '_blank')
        expect(link).toHaveAttribute('rel', 'noopener noreferrer')
      }
    })
  })

  describe('URL normalization', () => {
    it('normalizes partial URL with domain but no protocol', () => {
      render(<SocialLinks social={{ website: 'example.com/page' }} />)
      const link = screen.getByRole('link', { name: 'Website' })
      expect(link).toHaveAttribute('href', 'https://example.com/page')
    })

    it('passes a website value with no protocol through the fallback https prefix', () => {
      // The `website` row has `baseUrl: null`, so a bare token without a
      // protocol falls through to the final `https://` prefix branch.
      render(<SocialLinks social={{ website: 'example.com' }} />)
      const link = screen.getByRole('link', { name: 'Website' })
      expect(link).toHaveAttribute('href', 'https://example.com')
    })
  })

  describe('layout / styling', () => {
    it('applies custom className', () => {
      const { container } = render(
        <SocialLinks
          social={{ website: 'https://example.com' }}
          className="mt-4"
        />
      )
      expect(container.firstChild).toHaveClass('mt-4')
    })

    it('renders an icon (Lucide or custom SVG) inside each link', () => {
      render(<SocialLinks social={{ instagram: 'band', spotify: 'sp' }} />)

      for (const link of screen.getAllByRole('link')) {
        const icon = link.querySelector('svg')
        expect(icon).toBeInTheDocument()
        // Icons are aria-hidden so the sr-only label is the sole
        // accessible name.
        expect(icon).toHaveAttribute('aria-hidden', 'true')
      }
    })
  })

  // The read gate. One case per column, each written so deleting the gate fails
  // it: a conforming value keeps its link, and a legacy value on a host that is
  // not the platform's renders no link at all.
  describe('host-anchored read gate', () => {
    const conforming: Array<[string, string, string, string]> = [
      ['website', 'https://calexico.example.com/', 'Website', 'https://calexico.example.com/'],
      ['instagram', 'https://instagram.com/calexico', 'Instagram', 'https://instagram.com/calexico'],
      ['facebook', 'https://fb.com/calexico', 'Facebook', 'https://fb.com/calexico'],
      ['twitter', 'https://x.com/calexico', 'Twitter/X', 'https://x.com/calexico'],
      ['youtube', 'https://youtu.be/BLRUiVXeZKU', 'YouTube', 'https://youtu.be/BLRUiVXeZKU'],
      [
        'spotify',
        'https://open.spotify.com/artist/6cM2X9pUxJqL4z1234567',
        'Spotify',
        'https://open.spotify.com/artist/6cM2X9pUxJqL4z1234567',
      ],
      ['bandcamp', 'https://calexico.bandcamp.com', 'Bandcamp', 'https://calexico.bandcamp.com'],
      ['soundcloud', 'https://soundcloud.com/calexico', 'SoundCloud', 'https://soundcloud.com/calexico'],
    ]

    it.each(conforming)('renders %s on its own host', (key, value, label, href) => {
      render(<SocialLinks social={{ [key]: value }} />)
      expect(screen.getByRole('link', { name: label })).toHaveAttribute('href', href)
    })

    // The `website` column is deliberately absent here: it anchors no host, so
    // there is no off-platform value for it to refuse. Its rule is the scheme,
    // which the "never a javascript: href" case below covers.
    const offPlatform: Array<[string, string, string]> = [
      ['instagram', 'https://instagram.com.evil.test/calexico', 'Instagram'],
      ['facebook', 'https://notfacebook.com/calexico', 'Facebook'],
      ['twitter', 'https://evil.test/?x=twitter.com', 'Twitter/X'],
      ['youtube', 'https://youtube.com.evil.test/calexico', 'YouTube'],
      ['spotify', 'https://spotify-account-verify.evil.test/', 'Spotify'],
      ['bandcamp', 'https://evilbandcamp.com/calexico', 'Bandcamp'],
      ['soundcloud', 'https://soundcloud.com@evil.test/calexico', 'SoundCloud'],
    ]

    it.each(offPlatform)(
      'renders no link for a legacy %s value on %s',
      (key, value, label) => {
        render(<SocialLinks social={{ [key]: value }} />)
        expect(screen.queryByRole('link', { name: label })).not.toBeInTheDocument()
      }
    )

    it('renders nothing at all when every stored value is refused', () => {
      const { container } = render(
        <SocialLinks
          social={{
            spotify: 'https://spotify-account-verify.evil.test/',
            instagram: 'https://instagram.com.evil.test/x',
          }}
        />
      )
      expect(container.firstChild).toBeNull()
    })

    it('keeps the conforming links beside a refused one', () => {
      render(
        <SocialLinks
          social={{
            instagram: 'https://instagram.com/calexico',
            spotify: 'https://spotify-account-verify.evil.test/',
            bandcamp: 'https://calexico.bandcamp.com',
          }}
        />
      )
      expect(screen.getAllByRole('link')).toHaveLength(2)
      expect(screen.queryByRole('link', { name: 'Spotify' })).not.toBeInTheDocument()
    })

    it('never renders a javascript: href on any column', () => {
      render(
        <SocialLinks
          social={{
            website: 'javascript:alert(1)',
            instagram: 'javascript:alert(1)',
            spotify: 'javascript:alert(1)',
          }}
        />
      )
      expect(screen.queryAllByRole('link')).toHaveLength(0)
    })
  })
})
