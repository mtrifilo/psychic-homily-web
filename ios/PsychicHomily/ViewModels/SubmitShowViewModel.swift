import SwiftUI
import PhotosUI

@Observable
final class SubmitShowViewModel {
    // Image selection
    var selectedPhoto: PhotosPickerItem?
    var selectedImage: UIImage?
    var showCamera = false

    // Extraction state
    var isExtracting = false
    var extractionResult: ExtractedShowData?
    var extractionWarnings: [String]?
    var errorMessage: String?

    // Submission state
    var isSubmitting = false

    func processSelectedPhoto() async {
        guard let selectedPhoto else { return }

        do {
            if let data = try await selectedPhoto.loadTransferable(type: Data.self),
               let image = UIImage(data: data) {
                selectedImage = image
            }
        } catch {
            errorMessage = "Failed to load photo"
        }
    }

    func extractShowInfo() async {
        guard let image = selectedImage else {
            errorMessage = "No image selected"
            return
        }

        isExtracting = true
        errorMessage = nil
        extractionResult = nil

        // Compress and encode image
        guard let imageData = compressImage(image),
              let base64String = imageData.base64EncodedString() as String? else {
            errorMessage = "Failed to process image"
            isExtracting = false
            return
        }

        let request = ExtractShowRequest(
            type: "image",
            text: nil,
            imageData: base64String,
            mediaType: "image/jpeg"
        )

        do {
            let response: ExtractShowResponse = try await APIClient.shared.request(
                .aiProcessShow,
                body: request
            )

            if response.success, let data = response.data {
                extractionResult = data
                extractionWarnings = response.warnings
            } else {
                errorMessage = response.error ?? "Failed to extract show information"
            }
        } catch {
            errorMessage = error.localizedDescription
        }

        isExtracting = false
    }

    func reset() {
        selectedPhoto = nil
        selectedImage = nil
        extractionResult = nil
        extractionWarnings = nil
        errorMessage = nil
    }

    // MARK: - Image Compression

    private func compressImage(_ image: UIImage) -> Data? {
        // Resize to max 2048px on the long edge
        let maxDimension: CGFloat = 2048
        let size = image.size

        var newSize = size
        if size.width > maxDimension || size.height > maxDimension {
            let ratio = min(maxDimension / size.width, maxDimension / size.height)
            newSize = CGSize(width: size.width * ratio, height: size.height * ratio)
        }

        let renderer = UIGraphicsImageRenderer(size: newSize)
        let resized = renderer.image { _ in
            image.draw(in: CGRect(origin: .zero, size: newSize))
        }

        return resized.jpegData(compressionQuality: 0.8)
    }
}
