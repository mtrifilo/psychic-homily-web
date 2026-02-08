import SwiftUI
import PhotosUI

struct SubmitShowView: View {
    @State private var viewModel = SubmitShowViewModel()
    @Environment(AppState.self) private var appState

    var body: some View {
        ScrollView {
            VStack(spacing: 24) {
                if let result = viewModel.extractionResult {
                    // Extraction result → review & submit
                    ExtractionResultView(
                        data: result,
                        warnings: viewModel.extractionWarnings,
                        onSubmitted: { viewModel.reset() }
                    )
                } else {
                    // Image capture
                    ImageCaptureSection(viewModel: viewModel)
                }
            }
            .padding()
        }
        .navigationTitle("Submit Show")
        .onChange(of: viewModel.selectedPhoto) {
            Task {
                await viewModel.processSelectedPhoto()
                if viewModel.selectedImage != nil {
                    await viewModel.extractShowInfo()
                }
            }
        }
        .onChange(of: appState.pendingShareImage) {
            if appState.pendingShareImage {
                appState.pendingShareImage = false
                Task {
                    if let image = loadSharedImage() {
                        viewModel.selectedImage = image
                        await viewModel.extractShowInfo()
                    }
                }
            }
        }
    }

    private func loadSharedImage() -> UIImage? {
        guard let containerURL = FileManager.default.containerURL(
            forSecurityApplicationGroupIdentifier: "group.com.psychichomily"
        ) else { return nil }

        let fileURL = containerURL.appendingPathComponent("shared_image.jpg")
        guard let data = try? Data(contentsOf: fileURL) else { return nil }

        // Clean up after reading
        try? FileManager.default.removeItem(at: fileURL)

        return UIImage(data: data)
    }
}

// MARK: - Image Capture Section

private struct ImageCaptureSection: View {
    @Bindable var viewModel: SubmitShowViewModel

    var body: some View {
        VStack(spacing: 20) {
            // Header
            VStack(spacing: 8) {
                Image(systemName: "camera.viewfinder")
                    .font(.system(size: 56))
                    .foregroundStyle(.phSecondary)

                Text("Capture a Show Flyer")
                    .font(.title2)
                    .fontWeight(.bold)

                Text("Take a photo or select a screenshot of a show flyer. We'll extract the details automatically.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
            }
            .padding(.top, 20)

            // Image preview (if selected)
            if let image = viewModel.selectedImage {
                Image(uiImage: image)
                    .resizable()
                    .scaledToFit()
                    .frame(maxHeight: 300)
                    .clipShape(RoundedRectangle(cornerRadius: 12))
                    .overlay(
                        RoundedRectangle(cornerRadius: 12)
                            .stroke(.phSurface, lineWidth: 2)
                    )
            }

            // Extraction loading
            if viewModel.isExtracting {
                VStack(spacing: 12) {
                    ProgressView()
                        .tint(.phSecondary)
                    Text("Analyzing flyer...")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
                .padding()
            }

            // Error message
            if let error = viewModel.errorMessage {
                Text(error)
                    .font(.caption)
                    .foregroundStyle(.red)
                    .padding(.horizontal)
            }

            // Action buttons
            VStack(spacing: 12) {
                PhotosPicker(
                    selection: $viewModel.selectedPhoto,
                    matching: .images
                ) {
                    Label("Choose Photo", systemImage: "photo.on.rectangle")
                        .fontWeight(.semibold)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 4)
                }
                .buttonStyle(.borderedProminent)
                .tint(.phPrimary)

                Button {
                    viewModel.showCamera = true
                } label: {
                    Label("Take Photo", systemImage: "camera")
                        .fontWeight(.semibold)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 4)
                }
                .buttonStyle(.bordered)
                .tint(.phSecondary)
            }

            if viewModel.selectedImage != nil && !viewModel.isExtracting {
                Button {
                    Task { await viewModel.extractShowInfo() }
                } label: {
                    Text("Extract Show Info")
                        .fontWeight(.semibold)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 4)
                }
                .buttonStyle(.borderedProminent)
                .tint(.phAccent)
            }
        }
        .fullScreenCover(isPresented: $viewModel.showCamera) {
            CameraView(image: $viewModel.selectedImage)
        }
    }
}

// MARK: - Camera View (UIImagePickerController bridge)

struct CameraView: UIViewControllerRepresentable {
    @Binding var image: UIImage?
    @Environment(\.dismiss) private var dismiss

    func makeUIViewController(context: Context) -> UIImagePickerController {
        let picker = UIImagePickerController()
        picker.sourceType = .camera
        picker.delegate = context.coordinator
        return picker
    }

    func updateUIViewController(_ uiViewController: UIImagePickerController, context: Context) {}

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    class Coordinator: NSObject, UIImagePickerControllerDelegate, UINavigationControllerDelegate {
        let parent: CameraView

        init(_ parent: CameraView) {
            self.parent = parent
        }

        func imagePickerController(_ picker: UIImagePickerController, didFinishPickingMediaWithInfo info: [UIImagePickerController.InfoKey: Any]) {
            if let image = info[.originalImage] as? UIImage {
                parent.image = image
            }
            parent.dismiss()
        }

        func imagePickerControllerDidCancel(_ picker: UIImagePickerController) {
            parent.dismiss()
        }
    }
}

#Preview {
    NavigationStack {
        SubmitShowView()
    }
    .environment(AppState())
    .preferredColorScheme(.dark)
}
