import SwiftUI

@Observable
final class VenueDetailViewModel {
    var venue: Venue?
    var shows: [Show] = []
    var isLoading = false
    var errorMessage: String?

    func loadVenue(idOrSlug: String) async {
        isLoading = true
        errorMessage = nil

        do {
            async let venueResult: Venue = APIClient.shared.request(.getVenue(idOrSlug: idOrSlug))
            async let showsResult: VenueShowsResponse = APIClient.shared.request(.getVenueShows(idOrSlug: idOrSlug))

            venue = try await venueResult
            shows = try await showsResult.shows
        } catch {
            errorMessage = error.localizedDescription
        }

        isLoading = false
    }
}
