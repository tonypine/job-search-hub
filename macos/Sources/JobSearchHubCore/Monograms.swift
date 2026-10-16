import Foundation

/// A monogram: a company's or a person's initials on one of a few hues
/// picked by its name, so a name looks the same in every row, card and
/// inspector, across launches.
public enum Monograms {
    /// How many hues a monogram picks from; the app's palette has one per index.
    public static let hueCount = 6

    /// What a monogram shows for a name.
    public struct Look: Equatable, Sendable {
        public var letters: String
        public var hueIndex: Int
    }

    public static func getLook(for name: String) -> Look {
        Look(letters: getLetters(for: name), hueIndex: getHueIndex(for: name, count: hueCount))
    }

    /// "Alex Kim" reads "AK", "Northwind" "N"; each word gives its first
    /// letter or digit, a word without one is left out, and a name with
    /// none reads "?".
    public static func getLetters(for name: String) -> String {
        let initials = name.split(whereSeparator: \.isWhitespace).compactMap { word in word.first { $0.isLetter || $0.isNumber } }
        return initials.isEmpty ? "?" : String(initials.prefix(2)).uppercased()
    }

    /// The same name gets the same hue every launch, unlike `hashValue`,
    /// whatever its case or the spaces around it.
    public static func getHueIndex(for name: String, count: Int) -> Int {
        guard count > 0 else { return 0 }
        let key = name.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        return key.unicodeScalars.reduce(0) { ($0 &* 31 &+ Int($1.value)) & 0xFFFF } % count
    }
}
