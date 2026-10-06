import Foundation

/// The terminal's `hub`: `~/.local/bin/hub`, linked to the copy in the
/// installed app's bundle, so it is always the installed version's.
public enum HubCommandLink {
    public static func makeLinkURL(home: URL) -> URL {
        home.appending(path: ".local/bin/hub")
    }

    public enum State: Equatable, Sendable {
        /// Nothing at the link's path.
        case missing
        /// A link to the target.
        case linked
        /// Something else: a link elsewhere, with where it points, or a file.
        case other(destination: String?)
    }

    public static func readState(link: URL, target: URL, fileManager: FileManager = .default) -> State {
        // fileExists follows links, so a link whose target is gone would read as missing.
        guard (try? fileManager.attributesOfItem(atPath: link.path)) != nil else { return .missing }
        guard let destination = try? fileManager.destinationOfSymbolicLink(atPath: link.path) else {
            return .other(destination: nil)
        }
        return destination == target.path ? .linked : .other(destination: destination)
    }

    /// Links `link` to `target`, replacing whatever is there and creating its
    /// folder.
    public static func install(link: URL, target: URL, fileManager: FileManager = .default) throws {
        try fileManager.createDirectory(at: link.deletingLastPathComponent(), withIntermediateDirectories: true)
        if readState(link: link, target: target, fileManager: fileManager) != .missing {
            try fileManager.removeItem(at: link)
        }
        try fileManager.createSymbolicLink(atPath: link.path, withDestinationPath: target.path)
    }
}
