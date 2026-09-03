import Foundation

struct Show: Codable, Identifiable, Hashable, Sendable {
    static func == (lhs: Show, rhs: Show) -> Bool { lhs.id == rhs.id }
    func hash(into hasher: inout Hasher) { hasher.combine(id) }

    let id: Int
    let slug: String
    let title: String
    let eventDate: String
    let city: String
    let state: String
    /// The advance price when a door price is also recorded, otherwise the
    /// show's only price.
    let price: Double?
    /// The price at the door, recorded only when the show states one separately
    /// from `price`. Never derived from `price`.
    let doorPrice: Double?
    let ageRequirement: String?
    let description: String?
    let status: String
    let isSoldOut: Bool?
    let isCancelled: Bool?
    let submittedBy: Int?
    let artists: [ShowArtist]
    let venues: [ShowVenue]
    let createdAt: String
    let updatedAt: String

    enum CodingKeys: String, CodingKey {
        case id, slug, title, city, state, price, description, status, artists, venues
        case eventDate = "event_date"
        case doorPrice = "door_price"
        case ageRequirement = "age_requirement"
        case isSoldOut = "is_sold_out"
        case isCancelled = "is_cancelled"
        case submittedBy = "submitted_by"
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }

    /// Formatted event date for display using Arizona timezone
    var formattedDate: String {
        DateFormatting.displayDate(from: eventDate)
    }

    var formattedTime: String {
        DateFormatting.displayTime(from: eventDate)
    }

    var formattedDayOfWeek: String {
        DateFormatting.dayOfWeek(from: eventDate)
    }

    /// One amount as the site spells it: `Free` for zero, `$20` for a whole
    /// number, `$20.50` otherwise.
    static func formatPrice(_ amount: Double) -> String {
        if amount == 0 { return "Free" }
        if amount == amount.rounded() { return String(format: "$%.0f", amount) }
        return String(format: "$%.2f", amount)
    }

    /// The prices this show actually STATES, advance first: `[]`, `[20]`, or
    /// the pair `[20, 25]`.
    ///
    /// THE derivation of "what does this show cost", so a row and the detail
    /// screen cannot disagree about whether there are one or two numbers.
    ///
    /// An equal pair COLLAPSES to one: `$20/$20` spends two slots to say one
    /// thing and reads as a rendering bug. A lone DOOR price comes back
    /// indistinguishable from a lone advance price, deliberately — with one
    /// number there is nothing to tell it apart from, so qualifying it would
    /// add a word without adding a fact.
    ///
    /// Zero is a price, not silence, which is why the guards test for nil.
    var statedPrices: [Double] {
        if let price, let doorPrice, price != doorPrice {
            return [price, doorPrice]
        }
        if let only = price ?? doorPrice {
            return [only]
        }
        return []
    }

    /// The show is free to enter: the only price it states is zero.
    var isFree: Bool {
        statedPrices == [0]
    }

    /// A dense row's price: `$20`, `Free`, or the pair `$20/$25`.
    ///
    /// Both numbers rather than the advance half alone: a row showing `$20` for
    /// a show whose door is $25 tells a reader the wrong thing about money and
    /// they find out at the door.
    var priceText: String? {
        let prices = statedPrices
        if prices.isEmpty { return nil }
        return prices.map(Show.formatPrice).joined(separator: "/")
    }

    /// The detail screen's price: `$20`, `Free`, or the qualified pair
    /// `$20 ADV · DOOR $25`.
    ///
    /// `ADV` and `DOOR` are disambiguators, so they are spelled only when there
    /// are two different numbers to tell apart.
    var detailPriceText: String? {
        let prices = statedPrices
        if prices.count == 2 {
            return "\(Show.formatPrice(prices[0])) ADV · DOOR \(Show.formatPrice(prices[1]))"
        }
        return prices.first.map(Show.formatPrice)
    }

    var headliners: [ShowArtist] {
        artists.filter { $0.isHeadliner == true }
    }

    var openers: [ShowArtist] {
        artists.filter { $0.isHeadliner != true }
    }
}

struct ShowArtist: Codable, Identifiable, Hashable, Sendable {
    let id: Int
    let slug: String
    let name: String
    let isHeadliner: Bool?

    enum CodingKeys: String, CodingKey {
        case id, slug, name
        case isHeadliner = "is_headliner"
    }
}

struct ShowVenue: Codable, Identifiable, Hashable, Sendable {
    let id: Int
    let slug: String
    let name: String
    let address: String?
    let city: String
    let state: String
    let verified: Bool

    enum CodingKeys: String, CodingKey {
        case id, slug, name, address, city, state, verified
    }
}

struct UpcomingShowsResponse: Codable, Sendable {
    let shows: [Show]
    let timezone: String
    let pagination: PaginationMeta
}

struct PaginationMeta: Codable, Sendable {
    let nextCursor: String?
    let hasMore: Bool
    let limit: Int

    enum CodingKeys: String, CodingKey {
        case nextCursor = "next_cursor"
        case hasMore = "has_more"
        case limit
    }
}

struct ShowCitiesResponse: Codable, Sendable {
    let cities: [ShowCity]
}

struct ShowCity: Codable, Identifiable, Sendable, Hashable {
    let city: String
    let state: String
    let count: Int

    var id: String { "\(city)-\(state)" }
}
