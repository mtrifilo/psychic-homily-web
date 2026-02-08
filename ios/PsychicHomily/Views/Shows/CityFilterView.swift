import SwiftUI

struct CityFilterView: View {
    let cities: [ShowCity]
    let selectedCity: ShowCity?
    let onSelect: (ShowCity?) -> Void

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                FilterChip(
                    title: "All",
                    isSelected: selectedCity == nil,
                    action: { onSelect(nil) }
                )

                ForEach(cities) { city in
                    FilterChip(
                        title: "\(city.city) (\(city.count))",
                        isSelected: selectedCity == city,
                        action: { onSelect(city) }
                    )
                }
            }
            .padding(.horizontal)
            .padding(.vertical, 8)
        }
    }
}

struct FilterChip: View {
    let title: String
    let isSelected: Bool
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Text(title)
                .font(.caption)
                .fontWeight(isSelected ? .semibold : .regular)
                .padding(.horizontal, 14)
                .padding(.vertical, 8)
                .background(isSelected ? Color.phPrimary : Color.phSurface)
                .foregroundStyle(isSelected ? .white : .secondary)
                .clipShape(Capsule())
        }
    }
}
