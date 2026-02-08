import Foundation

struct ExtractShowRequest: Codable, Sendable {
    let type: String
    let text: String?
    let imageData: String?
    let mediaType: String?

    enum CodingKeys: String, CodingKey {
        case type, text
        case imageData = "image_data"
        case mediaType = "media_type"
    }
}

struct ExtractShowResponse: Codable, Sendable {
    let success: Bool
    let data: ExtractedShowData?
    let error: String?
    let warnings: [String]?
}

struct ExtractedShowData: Codable, Sendable {
    let artists: [ExtractedArtist]
    let venue: ExtractedVenue?
    let date: String?
    let time: String?
    let cost: String?
    let ages: String?
    let description: String?
}

struct ExtractedArtist: Codable, Identifiable, Sendable {
    let name: String
    let isHeadliner: Bool
    let matchedId: Int?
    let matchedName: String?
    let matchedSlug: String?
    let suggestions: [MatchSuggestion]?

    var id: String { name }

    var matchStatus: MatchStatus {
        if matchedId != nil { return .matched }
        if let suggestions, !suggestions.isEmpty { return .suggested }
        return .new
    }

    enum CodingKeys: String, CodingKey {
        case name
        case isHeadliner = "is_headliner"
        case matchedId = "matched_id"
        case matchedName = "matched_name"
        case matchedSlug = "matched_slug"
        case suggestions
    }
}

struct ExtractedVenue: Codable, Sendable {
    let name: String
    let city: String?
    let state: String?
    let matchedId: Int?
    let matchedName: String?
    let matchedSlug: String?
    let suggestions: [VenueMatchSuggestion]?

    var matchStatus: MatchStatus {
        if matchedId != nil { return .matched }
        if let suggestions, !suggestions.isEmpty { return .suggested }
        return .new
    }

    enum CodingKeys: String, CodingKey {
        case name, city, state
        case matchedId = "matched_id"
        case matchedName = "matched_name"
        case matchedSlug = "matched_slug"
        case suggestions
    }
}

struct MatchSuggestion: Codable, Identifiable, Sendable {
    let id: Int
    let name: String
    let slug: String
}

struct VenueMatchSuggestion: Codable, Identifiable, Sendable {
    let id: Int
    let name: String
    let slug: String
    let city: String
    let state: String
}

enum MatchStatus {
    case matched
    case suggested
    case new
}
