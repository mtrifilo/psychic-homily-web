import Foundation

enum HTTPMethod: String {
    case get = "GET"
    case post = "POST"
    case put = "PUT"
    case delete = "DELETE"
}

enum APIEndpoint {
    // Auth
    case login(email: String, password: String)
    case register(email: String, password: String, firstName: String?, lastName: String?)
    case appleCallback(identityToken: String, firstName: String?, lastName: String?)
    case getProfile
    case refreshToken
    case logout

    // Shows
    case upcomingShows(cursor: String?, limit: Int, city: String?, timezone: String)
    case showCities(timezone: String)
    case getShow(idOrSlug: String)
    case createShow(body: Data)
    case aiProcessShow

    // Artists
    case getArtist(idOrSlug: String)
    case getArtistShows(idOrSlug: String)
    case searchArtists(query: String)

    // Venues
    case getVenue(idOrSlug: String)
    case getVenueShows(idOrSlug: String)
    case searchVenues(query: String)

    // Saved Shows
    case getSavedShows
    case saveShow(showId: Int)
    case unsaveShow(showId: Int)
    case checkSaved(showId: Int)

    var path: String {
        switch self {
        case .login: return "/auth/login"
        case .register: return "/auth/register"
        case .appleCallback: return "/auth/apple/callback"
        case .getProfile: return "/auth/profile"
        case .refreshToken: return "/auth/refresh"
        case .logout: return "/auth/logout"

        case .upcomingShows: return "/shows/upcoming"
        case .showCities: return "/shows/cities"
        case .getShow(let id): return "/shows/\(id)"
        case .createShow(_): return "/shows"
        case .aiProcessShow: return "/shows/ai-process"

        case .getArtist(let id): return "/artists/\(id)"
        case .getArtistShows(let id): return "/artists/\(id)/shows"
        case .searchArtists: return "/artists/search"

        case .getVenue(let id): return "/venues/\(id)"
        case .getVenueShows(let id): return "/venues/\(id)/shows"
        case .searchVenues: return "/venues/search"

        case .getSavedShows: return "/saved-shows"
        case .saveShow(let id): return "/saved-shows/\(id)"
        case .unsaveShow(let id): return "/saved-shows/\(id)"
        case .checkSaved(let id): return "/saved-shows/\(id)/check"
        }
    }

    var method: HTTPMethod {
        switch self {
        case .login, .register, .appleCallback, .refreshToken, .logout,
             .createShow(_), .aiProcessShow, .saveShow:
            return .post
        case .unsaveShow:
            return .delete
        default:
            return .get
        }
    }

    var requiresAuth: Bool {
        switch self {
        case .login, .register, .appleCallback,
             .upcomingShows, .showCities, .getShow,
             .getArtist, .getArtistShows, .searchArtists,
             .getVenue, .getVenueShows, .searchVenues:
            return false
        default:
            return true
        }
    }

    var queryItems: [URLQueryItem]? {
        switch self {
        case .upcomingShows(let cursor, let limit, let city, let timezone):
            var items = [
                URLQueryItem(name: "timezone", value: timezone),
                URLQueryItem(name: "limit", value: String(limit)),
            ]
            if let cursor { items.append(URLQueryItem(name: "cursor", value: cursor)) }
            if let city { items.append(URLQueryItem(name: "city", value: city)) }
            return items

        case .showCities(let timezone):
            return [URLQueryItem(name: "timezone", value: timezone)]

        case .searchArtists(let query):
            return [URLQueryItem(name: "q", value: query)]

        case .searchVenues(let query):
            return [URLQueryItem(name: "q", value: query)]

        default:
            return nil
        }
    }

    var body: Encodable? {
        switch self {
        case .login(let email, let password):
            return LoginBody(email: email, password: password)
        case .register(let email, let password, let firstName, let lastName):
            return RegisterBody(email: email, password: password, firstName: firstName, lastName: lastName)
        case .appleCallback(let token, let firstName, let lastName):
            return AppleCallbackBody(identityToken: token, firstName: firstName, lastName: lastName)
        default:
            return nil
        }
    }

    /// Pre-serialized JSON body data (for endpoints that build JSON dynamically)
    var rawBody: Data? {
        switch self {
        case .createShow(let data):
            return data
        default:
            return nil
        }
    }
}

// MARK: - Request Bodies

private struct LoginBody: Encodable {
    let email: String
    let password: String
}

private struct RegisterBody: Encodable {
    let email: String
    let password: String
    let firstName: String?
    let lastName: String?

    enum CodingKeys: String, CodingKey {
        case email, password
        case firstName = "first_name"
        case lastName = "last_name"
    }
}

private struct AppleCallbackBody: Encodable {
    let identityToken: String
    let firstName: String?
    let lastName: String?

    enum CodingKeys: String, CodingKey {
        case identityToken = "identity_token"
        case firstName = "first_name"
        case lastName = "last_name"
    }
}
