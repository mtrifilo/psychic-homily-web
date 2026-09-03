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
    ///
    /// It must stay byte-identical to `formatPrice` in
    /// `frontend/lib/utils/formatters.ts` and `showPriceAmount` in
    /// `backend/internal/services/shared/show_price.go`, which render the same
    /// column to the same reader with no compiler holding the three together.
    /// The whole-number test is the part that drifts: rendering everything as
    /// whole dollars turns $12.50 into `$12` and a fifty-cent door into `$0`.
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
    /// MIRRORED in two other languages, which render the same column to the
    /// same reader with no compiler holding the three together:
    /// `statedShowPrices` in `frontend/lib/utils/showPrice.ts` and
    /// `ShowPriceText` in `backend/internal/services/shared/show_price.go`. A
    /// collapse rule changed here needs the same change in both.
    ///
    /// An equal pair COLLAPSES to one: two slots and a separator to say one
    /// thing reads as a rendering bug. A lone DOOR price comes back
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
        let prices = statedPrices
        return prices.count == 1 && prices[0] == 0
    }

    /// A dense row's price: `$20`, `Free`, or the pair `$20/$25`.
    ///
    /// Both numbers rather than the advance half alone, so a row never quotes
    /// the advance price of a show whose door price is higher.
    var priceText: String? {
        let prices = statedPrices
        if prices.isEmpty { return nil }
        return prices.map(Show.formatPrice).joined(separator: "/")
    }

    /// How a screen reader should say the row's price, or nil when the visible
    /// text already reads correctly.
    ///
    /// Present ONLY for a pair: `$20/$25` is announced as "twenty slash
    /// twenty-five", a fact about money read out as punctuation. A lone price
    /// has nothing to be disambiguated from, and labelling it would make the
    /// reader say it twice.
    var priceAccessibilityLabel: String? {
        let prices = statedPrices
        guard prices.count == 2 else { return nil }
        return "\(Show.formatPrice(prices[0])) advance, \(Show.formatPrice(prices[1])) at the door"
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
