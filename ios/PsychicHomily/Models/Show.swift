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
    let price: Double?
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

    var priceText: String? {
        guard let price else { return nil }
        if price == 0 { return "Free" }
        return String(format: "$%.0f", price)
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
