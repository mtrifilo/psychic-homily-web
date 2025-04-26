import bands from './bands.json'
import venues from './venues.json'

export interface Band {
    name: string
    social?: {
        instagram?: string
    }
    url?: string
    arizona_band?: boolean
}

export interface Venue {
    name: string
    address?: string
    city: string
    state: string
    zip?: string
    social?: {
        instagram?: string
        website?: string
    }
}

export { bands, venues }
