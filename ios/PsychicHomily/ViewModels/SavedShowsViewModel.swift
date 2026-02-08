import SwiftUI

@Observable
final class SavedShowsViewModel {
    var shows: [Show] = []
    var isLoading = false
    var errorMessage: String?

    func loadSavedShows() async {
        isLoading = true
        errorMessage = nil

        do {
            let response: SavedShowsResponse = try await APIClient.shared.request(.getSavedShows)
            shows = response.shows
        } catch {
            errorMessage = error.localizedDescription
        }

        isLoading = false
    }

    func unsaveShow(_ showId: Int) async {
        // Optimistic removal
        shows.removeAll { $0.id == showId }

        do {
            try await APIClient.shared.requestVoid(.unsaveShow(showId: showId))
        } catch {
            // Reload on failure to restore correct state
            await loadSavedShows()
        }
    }
}
