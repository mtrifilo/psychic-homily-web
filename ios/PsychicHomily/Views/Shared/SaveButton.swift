import SwiftUI

struct SaveButton: View {
    let isSaved: Bool
    let isSaving: Bool
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: isSaved ? "bookmark.fill" : "bookmark")
                .foregroundStyle(isSaved ? .phAccent : .secondary)
                .font(.title3)
                .contentTransition(.symbolEffect(.replace))
        }
        .disabled(isSaving)
        .sensoryFeedback(isSaved ? .success : .selection, trigger: isSaved)
    }
}

#Preview {
    HStack(spacing: 24) {
        SaveButton(isSaved: false, isSaving: false) {}
        SaveButton(isSaved: true, isSaving: false) {}
    }
    .preferredColorScheme(.dark)
}
