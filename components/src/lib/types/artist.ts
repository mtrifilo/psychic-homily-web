export interface Artist {
    id: number
    name: string
    state: string | null
    city: string | null
    social: ArtistSocial
    created_at: string
    updated_at: string
}

export interface ArtistSocial {
    instagram: string | null
    facebook: string | null
    twitter: string | null
    youtube: string | null
    spotify: string | null
    soundcloud: string | null
    bandcamp: string | null
    website: string | null
}

export interface ArtistSearchParams {
    query: string
}

export interface ArtistSearchResponse {
    artists: Artist[]
    count: number
}

export const getArtistLocation = (artist: Artist): string => {
    const parts = [artist.city, artist.state].filter(Boolean)
    return parts.length > 0 ? parts.join(', ') : 'Location Unknown'
}
