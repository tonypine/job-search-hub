import Foundation

/// The time axis under a window's charts: the instants it labels and what
/// each label reads. A label starts at its instant's gridline and reads to
/// its right, as Swift Charts sets a date's, so the labels are as close as
/// the plot lets each read in full, and the last is a label's width from
/// the plot's end, where it would be cut short. A window of a day or so
/// reads in hours, each midnight as its date; a longer one in days, Mondays
/// or months. What it reads follows the window's span, not its bucket, so a
/// day counted by the day doesn't read its date under every gridline.
public enum RunTimeAxis {
    public struct Tick: Equatable, Sendable {
        public var date: Date
        /// "8 PM", "Oct 20", "Oct 2026".
        public var title: String
    }

    /// The room a label takes right of its gridline, in points, the widest
    /// of them included: "Sep 2026".
    public static let labelWidth: Double = 56
    /// The least room between two labels, in points.
    public static let labelGap: Double = 16

    /// The labelled instants of a window drawn on a plot this many points
    /// wide, read in the calendar's time zone and locale.
    public static func getTicks(from: Date, to: Date, width: Double, calendar: Calendar) -> [Tick] {
        let span = to.timeIntervalSince(from)
        guard span > 0, width > 0 else { return [] }
        let pointsPerSecond = width / span
        let leastApart = (labelWidth + labelGap) / pointsPerSecond
        guard let stride = Stride.all.first(where: { $0.shortest >= leastApart }) else { return [] }
        let lastRoom = labelWidth / pointsPerSecond
        return stride.getDates(from: from, to: to, calendar: calendar)
            .filter { $0 >= from && to.timeIntervalSince($0) >= lastRoom }
            .map { Tick(date: $0, title: stride.describe($0, calendar: calendar)) }
    }

    /// How far apart the labelled instants are: so many hours from midnight,
    /// days from the window's first, weeks from a Monday, or months from
    /// January.
    private enum Stride {
        case hours(Int), days(Int), weeks(Int), months(Int)

        static let all: [Stride] = [
            .hours(1), .hours(2), .hours(3), .hours(4), .hours(6), .hours(12),
            .days(1), .days(2), .weeks(1), .weeks(2),
            .months(1), .months(2), .months(3), .months(6), .months(12), .months(24), .months(60),
        ]

        private static let hour: TimeInterval = 60 * 60

        /// The least time between two of its instants: a day is 23 hours
        /// across a daylight-saving change, a month as short as 28 days.
        var shortest: TimeInterval {
            switch self {
            case let .hours(count): Double(count) * Self.hour
            case let .days(count): Double(count) * 23 * Self.hour
            case let .weeks(count): Double(count) * (7 * 24 - 1) * Self.hour
            case let .months(count): Double(count) * 28 * 24 * Self.hour
            }
        }

        /// Its instants from the window's first midnight, or the week's or
        /// month's start before it, up to the window's end.
        func getDates(from: Date, to: Date, calendar: Calendar) -> [Date] {
            let midnight = calendar.startOfDay(for: from)
            let first: Date?
            let step: (Date) -> Date?
            switch self {
            case let .hours(count):
                // Every hour, kept when it's a whole number of strides past
                // midnight, so a short night or a long one keeps the clock's.
                return Swift.stride(from: midnight, through: to, by: Self.hour)
                    .filter { calendar.component(.hour, from: $0) % count == 0 && calendar.component(.minute, from: $0) == 0 }
            case let .days(count):
                first = midnight
                step = { calendar.date(byAdding: .day, value: count, to: $0) }
            case let .weeks(count):
                // The hub's weeks start on Monday.
                let sinceMonday = (calendar.component(.weekday, from: midnight) + 5) % 7
                first = calendar.date(byAdding: .day, value: -sinceMonday, to: midnight)
                step = { calendar.date(byAdding: .day, value: 7 * count, to: $0) }
            case let .months(count):
                let monthStart = calendar.dateInterval(of: .month, for: from)?.start ?? midnight
                let sinceJanuary = (calendar.component(.month, from: monthStart) - 1) % count
                first = calendar.date(byAdding: .month, value: -sinceJanuary, to: monthStart)
                step = { calendar.date(byAdding: .month, value: count, to: $0) }
            }
            var dates: [Date] = []
            var next = first
            while let date = next, date <= to {
                dates.append(date)
                next = step(date).flatMap { $0 > date ? $0 : nil }
            }
            return dates
        }

        /// "8 PM", or "Oct 20" at midnight, for hours; "Oct 20" for days and
        /// weeks; "Oct 2026" for months.
        func describe(_ date: Date, calendar: Calendar) -> String {
            var style: Date.FormatStyle
            switch self {
            case .hours where calendar.component(.hour, from: date) != 0:
                style = .dateTime.hour()
            case .hours, .days, .weeks:
                style = .dateTime.month(.abbreviated).day()
            case .months:
                style = .dateTime.month(.abbreviated).year()
            }
            style.calendar = calendar
            style.timeZone = calendar.timeZone
            style.locale = calendar.locale ?? .current
            return date.formatted(style)
        }
    }
}
