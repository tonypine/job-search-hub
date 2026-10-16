import CryptoKit
import Foundation

/// Why a downloaded version wasn't offered. Each check names its own, so
/// Settings › Version can say which one it failed.
public enum DownloadProblem: Error, Equatable, Sendable {
    /// The release has no zip, or no checksum beside it.
    case missingAssets
    case downloadFailed(String)
    /// The `.sha256` asset isn't `<hex>  <file>`.
    case unreadableChecksum
    case checksumMismatch(expected: String, found: String)
    /// The zip unpacked to no app, or `ditto` failed.
    case noApp(String)
    /// `codesign --verify --strict --deep` refused it, with what it said.
    case invalidSignature(String)
    /// The running app carries no team, as a build signed ad hoc, so nothing
    /// can be matched to it.
    case runningAppUnsigned
    case otherTeam(found: String?, expected: String)
    case otherRequirement
    /// The version in the place named (`Info.plist`, `hub-server --version`)
    /// isn't the one the release named.
    case otherVersion(place: String, found: String?, expected: String)

    /// Which check it failed, in a few words.
    public var title: String {
        switch self {
        case .missingAssets, .downloadFailed: "couldn't be downloaded"
        case .unreadableChecksum, .checksumMismatch: "failed its checksum"
        case .noApp: "couldn't be unpacked"
        case .invalidSignature: "failed its signature check"
        case .runningAppUnsigned, .otherTeam, .otherRequirement: "isn't signed like this app"
        case .otherVersion: "carries another version"
        }
    }

    /// What happened, in a sentence that ends with what the app did about it.
    public var explanation: String {
        switch self {
        case .missingAssets:
            "Its release has no app or checksum to download, so it wasn't offered."
        case let .downloadFailed(reason):
            "The download failed (\(reason)), so it wasn't offered. The next check tries again."
        case .unreadableChecksum:
            "Its checksum file couldn't be read, so it was deleted and not offered."
        case .checksumMismatch:
            "The download doesn't match its checksum, so it was deleted and not offered."
        case .noApp:
            "The download held no app, so it was deleted and not offered."
        case .invalidSignature:
            "Its signature doesn't verify, so it was deleted and not offered."
        case .runningAppUnsigned:
            "This app isn't signed by a team, so no download can be matched to it. Install a signed build first."
        case .otherTeam:
            "Its signature isn't from your team, so it was deleted and not offered."
        case .otherRequirement:
            "Its signature doesn't meet this app's designated requirement, so it was deleted and not offered."
        case let .otherVersion(place, found, expected):
            "Its \(place) says \(found ?? "nothing"), not \(expected), so it was deleted and not offered."
        }
    }
}

/// A `.sha256` asset, as `shasum -a 256` writes it: the hex digest, two
/// spaces, the file's name.
public enum Checksum {
    /// The digest, lowercased, or nil when the text holds none.
    public static func parse(_ text: String) -> String? {
        guard let digest = text.split(whereSeparator: \.isWhitespace).first?.lowercased(),
              digest.count == 64, digest.allSatisfy(\.isHexDigit)
        else { return nil }
        return digest
    }

    /// The file's SHA-256, in lowercase hex, read a megabyte at a time.
    public static func digest(of file: URL) throws -> String {
        let handle = try FileHandle(forReadingFrom: file)
        defer { try? handle.close() }
        var hasher = SHA256()
        while let chunk = try handle.read(upToCount: 1 << 20), !chunk.isEmpty {
            hasher.update(data: chunk)
        }
        return hasher.finalize().map { String(format: "%02x", $0) }.joined()
    }

    /// Check 1: the zip matches its `.sha256` asset.
    public static func verify(_ file: URL, against checksumText: String) throws(DownloadProblem) {
        guard let expected = parse(checksumText) else { throw .unreadableChecksum }
        let found: String
        do {
            found = try digest(of: file)
        } catch {
            throw .downloadFailed(error.localizedDescription)
        }
        guard found == expected else { throw .checksumMismatch(expected: expected, found: found) }
    }
}

/// Who signed a bundle: its team, and its designated requirement, the rule
/// macOS (and the Keychain) uses to tell one app from another.
public struct BundleSigning: Equatable, Sendable {
    public var teamID: String?
    public var designatedRequirement: String?

    public init(teamID: String?, designatedRequirement: String?) {
        self.teamID = teamID
        self.designatedRequirement = designatedRequirement
    }

    /// Reads `codesign --display --verbose=2` and `codesign --display
    /// --requirements -`, which print to standard error. An ad hoc signature
    /// has `TeamIdentifier=not set`.
    public static func parse(details: String, requirements: String) -> BundleSigning {
        let team = details.components(separatedBy: .newlines)
            .first { $0.hasPrefix("TeamIdentifier=") }
            .map { String($0.dropFirst("TeamIdentifier=".count)).trimmingCharacters(in: .whitespaces) }
        let requirement = requirements.components(separatedBy: .newlines)
            .first { $0.hasPrefix("designated => ") }
            .map { String($0.dropFirst("designated => ".count)).trimmingCharacters(in: .whitespaces) }
        return BundleSigning(
            teamID: team == nil || team == "not set" || team?.isEmpty == true ? nil : team,
            designatedRequirement: requirement?.isEmpty == true ? nil : requirement
        )
    }
}

/// A server's build, as `hub-server --version` prints it and `GET
/// /v1/version` answers:
///
///     hub-server 0.1.252
///     commit 4c1e8a9f00d1b2c3
///     newest migration 91
public struct ServerBuild: Equatable, Sendable, Decodable {
    public var version: String
    public var newestMigration: Int?

    public init(version: String, newestMigration: Int?) {
        self.version = version
        self.newestMigration = newestMigration
    }

    public static func parse(_ output: String) -> ServerBuild? {
        var version: String?
        var migration: Int?
        for line in output.components(separatedBy: .newlines) {
            if line.hasPrefix("hub-server ") {
                version = String(line.dropFirst("hub-server ".count)).trimmingCharacters(in: .whitespaces)
            } else if line.hasPrefix("newest migration ") {
                migration = Int(line.dropFirst("newest migration ".count).trimmingCharacters(in: .whitespaces))
            }
        }
        return version.map { ServerBuild(version: $0, newestMigration: migration) }
    }
}

/// What the checks read from a bundle on disk. The app's reads run
/// `codesign` and the bundle's `hub-server`; tests answer for them.
public protocol BundleInspecting: Sendable {
    /// Nil when `codesign --verify --strict --deep` accepts the bundle;
    /// otherwise what it said.
    func verifySignature(of app: URL) async -> String?
    func readSigning(of app: URL) async -> BundleSigning
    /// The bundle's `hub-server --version`, or nil without a server.
    func readServerBuild(of app: URL) async -> ServerBuild?
}

/// A downloaded version that passed every check.
public struct CheckedVersion: Equatable, Sendable {
    public var version: HubVersion
    public var app: URL
    /// Whether installing it migrates the database; nil when the running
    /// server's newest migration couldn't be read.
    public var changesDatabase: Bool?

    public init(version: HubVersion, app: URL, changesDatabase: Bool?) {
        self.version = version
        self.app = app
        self.changesDatabase = changesDatabase
    }
}

/// Checks 2 to 5 on an unpacked download, in order, after the checksum:
/// the signature verifies; it's signed by the running app's team with the
/// same designated requirement; it carries the release's version in its
/// `Info.plist` and its `hub-server --version`; and its newest migration
/// against the running server's says whether it changes the database. A
/// tampered download can't pass without the owner's private key.
public enum BundleChecks {
    public static func check(
        _ app: URL, expected version: HubVersion, running: BundleSigning, runningServer: ServerBuild?,
        inspector: some BundleInspecting
    ) async throws(DownloadProblem) -> CheckedVersion {
        if let refusal = await inspector.verifySignature(of: app) {
            throw .invalidSignature(refusal)
        }

        guard let runningTeam = running.teamID, let runningRequirement = running.designatedRequirement else {
            throw .runningAppUnsigned
        }
        let signing = await inspector.readSigning(of: app)
        guard signing.teamID == runningTeam else { throw .otherTeam(found: signing.teamID, expected: runningTeam) }
        guard signing.designatedRequirement == runningRequirement else { throw .otherRequirement }

        let expected = version.description
        for key in ["CFBundleShortVersionString", "CFBundleVersion"] {
            let found = readInfo(key, of: app)
            guard found == expected else { throw .otherVersion(place: "Info.plist's \(key)", found: found, expected: expected) }
        }
        let server = await inspector.readServerBuild(of: app)
        guard server?.version == expected else {
            throw .otherVersion(place: "hub-server --version", found: server?.version, expected: expected)
        }

        var changesDatabase: Bool?
        if let newest = server?.newestMigration, let current = runningServer?.newestMigration {
            changesDatabase = newest > current
        }
        return CheckedVersion(version: version, app: app, changesDatabase: changesDatabase)
    }

    /// A key of the bundle's `Contents/Info.plist`, read from the file.
    public static func readInfo(_ key: String, of app: URL) -> String? {
        guard let data = try? Data(contentsOf: app.appending(path: "Contents/Info.plist")),
              let plist = try? PropertyListSerialization.propertyList(from: data, format: nil) as? [String: Any]
        else { return nil }
        return plist[key] as? String
    }
}

/// The checks' reads on a real bundle, through `/usr/bin/codesign` and the
/// bundle's own `hub-server`.
public struct CodesignInspector: BundleInspecting {
    public init() {}

    public func verifySignature(of app: URL) async -> String? {
        let result = await ProcessRunner.run(URL(filePath: "/usr/bin/codesign"), arguments: ["--verify", "--strict", "--deep", app.path])
        guard result.status != 0 else { return nil }
        let output = result.output.trimmingCharacters(in: .whitespacesAndNewlines)
        return output.isEmpty ? "codesign exited with \(result.status)" : output
    }

    public func readSigning(of app: URL) async -> BundleSigning {
        let codesign = URL(filePath: "/usr/bin/codesign")
        async let details = ProcessRunner.run(codesign, arguments: ["--display", "--verbose=2", app.path])
        async let requirements = ProcessRunner.run(codesign, arguments: ["--display", "--requirements", "-", app.path])
        return await BundleSigning.parse(details: details.output, requirements: requirements.output)
    }

    public func readServerBuild(of app: URL) async -> ServerBuild? {
        let server = ServerLaunchAgent.makeCommandURL("hub-server", bundle: app)
        guard FileManager.default.isExecutableFile(atPath: server.path) else { return nil }
        let result = await ProcessRunner.run(server, arguments: ["--version"])
        return result.status == 0 ? ServerBuild.parse(result.output) : nil
    }
}

/// Runs a command to the end and returns what it printed, both streams
/// together, and how it exited. The output is read while the command runs,
/// so one that prints more than a pipe holds doesn't stall.
public enum ProcessRunner {
    public static func run(_ command: URL, arguments: [String]) async -> (output: String, status: Int32) {
        await Task.detached {
            let process = Process()
            process.executableURL = command
            process.arguments = arguments
            let output = Pipe()
            process.standardOutput = output
            process.standardError = output
            do {
                try process.run()
            } catch {
                return (error.localizedDescription, -1)
            }
            let data = output.fileHandleForReading.readDataToEndOfFile()
            process.waitUntilExit()
            return (String(decoding: data, as: UTF8.self), process.terminationStatus)
        }.value
    }
}
