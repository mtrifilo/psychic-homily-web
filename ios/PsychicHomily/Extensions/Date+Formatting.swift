import Foundation

enum DateFormatting {
    /// Arizona timezone — no DST, always MST (UTC-7)
    static let arizonaTimeZone = TimeZone(identifier: "America/Phoenix")!

    private static let isoParser: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    private static let fallbackParser: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f
    }()

    private static let displayDateFormatter: DateFormatter = {
        let f = DateFormatter()
        f.timeZone = arizonaTimeZone
        f.dateFormat = "MMM d, yyyy"
        return f
    }()

    private static let displayTimeFormatter: DateFormatter = {
        let f = DateFormatter()
        f.timeZone = arizonaTimeZone
        f.dateFormat = "h:mm a"
        return f
    }()

    private static let dayOfWeekFormatter: DateFormatter = {
        let f = DateFormatter()
        f.timeZone = arizonaTimeZone
        f.dateFormat = "EEEE"
        return f
    }()

    private static let shortDateFormatter: DateFormatter = {
        let f = DateFormatter()
        f.timeZone = arizonaTimeZone
        f.dateFormat = "EEE, MMM d"
        return f
    }()

    static func parse(_ isoString: String) -> Date? {
        isoParser.date(from: isoString) ?? fallbackParser.date(from: isoString)
    }

    static func displayDate(from isoString: String) -> String {
        guard let date = parse(isoString) else { return isoString }
        return displayDateFormatter.string(from: date)
    }

    static func displayTime(from isoString: String) -> String {
        guard let date = parse(isoString) else { return "" }
        return displayTimeFormatter.string(from: date)
    }

    static func dayOfWeek(from isoString: String) -> String {
        guard let date = parse(isoString) else { return "" }
        return dayOfWeekFormatter.string(from: date)
    }

    static func shortDate(from isoString: String) -> String {
        guard let date = parse(isoString) else { return isoString }
        return shortDateFormatter.string(from: date)
    }
}
