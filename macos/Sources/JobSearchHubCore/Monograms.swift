import Foundation

/// The tile before a company's or a person's name: the first letters of
/// its first two words, on one of a few hues picked by the name, so a name
/// keeps its look across pages and launches.
public enum Monograms {
    /// "Northwind" reads "N", "Alex Kim" "AK". Each word gives its first
    /// letter, or its first digit when it has no letter, and a word with
    /// neither is left out; a name with none reads "?".
    public static func getLetters(for name: String) -> String {
        let letters = name.split(whereSeparator: \.isWhitespace).compactMap { word in
            word.first(where: \.isLetter) ?? word.first(where: \.isNumber)
        }
        return letters.isEmpty ? "?" : String(letters.prefix(2)).uppercased()
    }

    /// The same name gets the same hue every launch, unlike `hashValue`.
    public static func getHueIndex(for name: String, count: Int) -> Int {
        guard count > 0 else { return 0 }
        return name.lowercased().unicodeScalars.reduce(0) { ($0 &* 31 &+ Int($1.value)) & 0xFFFF } % count
    }
}
