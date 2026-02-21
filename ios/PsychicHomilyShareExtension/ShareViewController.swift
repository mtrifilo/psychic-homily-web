import UIKit
import UniformTypeIdentifiers

class ShareViewController: UIViewController {
    override func viewDidLoad() {
        super.viewDidLoad()
        handleSharedImage()
    }

    private func handleSharedImage() {
        guard let extensionContext,
              let inputItem = extensionContext.inputItems.first as? NSExtensionItem,
              let itemProvider = inputItem.attachments?.first else {
            completeRequest()
            return
        }

        let imageType = UTType.image.identifier

        guard itemProvider.hasItemConformingToTypeIdentifier(imageType) else {
            completeRequest()
            return
        }

        itemProvider.loadItem(forTypeIdentifier: imageType, options: nil) { [weak self] item, error in
            guard let self else { return }

            var imageData: Data?

            if let url = item as? URL {
                imageData = try? Data(contentsOf: url)
            } else if let data = item as? Data {
                imageData = data
            } else if let image = item as? UIImage {
                imageData = image.jpegData(compressionQuality: 0.8)
            }

            guard let imageData else {
                DispatchQueue.main.async { self.completeRequest() }
                return
            }

            // Write to App Group shared container
            let saved = self.saveToAppGroup(imageData)

            DispatchQueue.main.async {
                if saved, let appURL = URL(string: "psychichomily://submit?source=share") {
                    // Use extensionContext.open() to open the containing app
                    self.extensionContext?.open(appURL) { _ in
                        self.completeRequest()
                    }
                } else {
                    self.completeRequest()
                }
            }
        }
    }

    private func saveToAppGroup(_ data: Data) -> Bool {
        guard let containerURL = FileManager.default.containerURL(
            forSecurityApplicationGroupIdentifier: "group.com.psychichomily"
        ) else {
            return false
        }

        let fileURL = containerURL.appendingPathComponent("shared_image.jpg")
        do {
            try data.write(to: fileURL)
            return true
        } catch {
            return false
        }
    }

    private func completeRequest() {
        extensionContext?.completeRequest(returningItems: nil)
    }
}
