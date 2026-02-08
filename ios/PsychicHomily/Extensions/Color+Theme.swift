import SwiftUI

extension Color {
    /// Brand colors for Psychic Homily
    static let phPrimary = Color(hex: 0x6D28D9)     // Purple
    static let phSecondary = Color(hex: 0xA855F7)    // Light purple
    static let phBackground = Color(hex: 0x0A0A0A)   // Near-black
    static let phSurface = Color(hex: 0x1A1A1A)      // Dark surface
    static let phAccent = Color(hex: 0xF59E0B)        // Amber accent
}

extension Color {
    init(hex: UInt, alpha: Double = 1.0) {
        self.init(
            .sRGB,
            red: Double((hex >> 16) & 0xFF) / 255.0,
            green: Double((hex >> 8) & 0xFF) / 255.0,
            blue: Double(hex & 0xFF) / 255.0,
            opacity: alpha
        )
    }
}
