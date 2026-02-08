import SwiftUI

@Observable
final class ShowDetailViewModel {
    var show: Show?
    var isSaved = false
    var isLoading = false
    var isSaving = false
    var errorMessage: String?

    func loadShow(idOrSlug: String) async {
        isLoading = true
        errorMessage = nil

        do {
            show = try await APIClient.shared.request(.getShow(idOrSlug: idOrSlug))
        } catch {
            errorMessage = error.localizedDescription
        }

        isLoading = false
    }

    func checkSaved(showId: Int) async {
        guard KeychainManager.getToken() != nil else { return }

        do {
            let response: CheckSavedResponse = try await APIClient.shared.request(
                .checkSaved(showId: showId)
            )
            isSaved = response.saved
        } catch {
            // Non-critical
        }
    }

    func toggleSave(showId: Int) async {
        isSaving = true

        let wasSaved = isSaved
        isSaved.toggle() // Optimistic update

        do {
            if wasSaved {
                try await APIClient.shared.requestVoid(.unsaveShow(showId: showId))
            } else {
                let _: SaveShowResponse = try await APIClient.shared.request(.saveShow(showId: showId))
            }
        } catch {
            isSaved = wasSaved // Revert on failure
        }

        isSaving = false
    }
}
