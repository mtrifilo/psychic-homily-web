import SwiftUI

struct StatusBadge: View {
    let text: String
    let color: Color

    var body: some View {
        Text(text)
            .font(.caption2)
            .fontWeight(.semibold)
            .textCase(.uppercase)
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .background(color.opacity(0.2))
            .foregroundStyle(color)
            .clipShape(Capsule())
    }
}

extension StatusBadge {
    static func soldOut() -> StatusBadge {
        StatusBadge(text: "Sold Out", color: .red)
    }

    static func cancelled() -> StatusBadge {
        StatusBadge(text: "Cancelled", color: .orange)
    }

    static func free() -> StatusBadge {
        StatusBadge(text: "Free", color: .green)
    }

    static func verified() -> StatusBadge {
        StatusBadge(text: "Verified", color: .blue)
    }
}

#Preview {
    HStack {
        StatusBadge.soldOut()
        StatusBadge.cancelled()
        StatusBadge.free()
        StatusBadge.verified()
    }
    .preferredColorScheme(.dark)
}
