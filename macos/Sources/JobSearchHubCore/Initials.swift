import Foundation

/// The letters of a monogram: a company's or a person's first two words.
public enum Initials {
    /// "Alex Kim" reads "AK", "Northwind" "N"; each word gives its first
    /// letter or digit, and a word without one is left out.
    public static func make(from name: String) -> String {
        let words = name.split(whereSeparator: \.isWhitespace).compactMap { word in word.first { $0.isLetter || $0.isNumber } }
        return String(words.prefix(2)).uppercased()
    }
}
