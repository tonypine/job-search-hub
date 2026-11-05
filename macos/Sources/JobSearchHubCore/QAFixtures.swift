import Foundation

/// QA mode: a QA build, which make-app.sh marks with HubQABuild, or a launch
/// with `--qa-mode`.
public enum QAMode {
    public static func isOn(arguments: [String], isQABuild: Bool) -> Bool {
        isQABuild || arguments.contains(HubConnection.qaModeArgument)
    }

    /// This launch's.
    public static let current = isOn(
        arguments: ProcessInfo.processInfo.arguments,
        isQABuild: Bundle.main.object(forInfoDictionaryKey: HubConnection.qaBuildInfoKey) as? Bool == true
    )
}

/// The folder Symphony's QA launch copies to the QA host and names in
/// `SYMPHONY_BAR_QA_API_FIXTURES`, read in QA mode only, so QA can open the
/// screens that need a past install. It may hold:
///
/// - `updates/`: the app's Updates folder, in place of
///   `~/Library/Application Support/JobSearchHub/Updates`, with the version
///   kept in `previous/` and the install records;
/// - `running-version`: the version the app runs as, such as `0.1.252`, in
///   place of the build's own (`0.1.0-dev` in QA), so the one in `previous/`
///   is older;
/// - `releases.json`: the Mac releases in GitHub's shape, the feed read when
///   neither `HUB_RELEASES_URL` nor the build names one.
///
/// The app writes only inside the copy: an install, or going back, with a
/// fixture's Updates folder is recorded in its install log and never runs
/// `hub-update`.
public struct QAFixtures: Equatable, Sendable {
    public static let directoryVariable = "SYMPHONY_BAR_QA_API_FIXTURES"
    public static let updatesName = "updates"
    public static let runningVersionName = "running-version"
    public static let releasesName = "releases.json"

    public let directory: URL

    public init(directory: URL) {
        self.directory = directory
    }

    /// The fixture the variable names, in QA mode; nil outside it, and when
    /// the variable is unset or empty.
    public static func find(isQAMode: Bool, environment: [String: String]) -> QAFixtures? {
        guard isQAMode, let path = environment[directoryVariable]?.trimmingCharacters(in: .whitespacesAndNewlines), !path.isEmpty
        else { return nil }
        return QAFixtures(directory: URL(filePath: path, directoryHint: .isDirectory))
    }

    /// This launch's.
    public static let current = find(isQAMode: QAMode.current, environment: ProcessInfo.processInfo.environment)

    /// `updates/`, when the fixture holds that folder.
    public var updatesRoot: URL? {
        let url = directory.appending(path: Self.updatesName, directoryHint: .isDirectory)
        var isDirectory: ObjCBool = false
        guard FileManager.default.fileExists(atPath: url.path, isDirectory: &isDirectory), isDirectory.boolValue else { return nil }
        return url
    }

    /// The version `running-version` names, when it holds one.
    public func readRunningVersion() -> HubVersion? {
        guard let text = try? String(contentsOf: directory.appending(path: Self.runningVersionName), encoding: .utf8) else { return nil }
        return HubVersion(text.trimmingCharacters(in: .whitespacesAndNewlines))
    }

    /// `releases.json`, when the fixture holds it.
    public var releasesURL: URL? {
        let url = directory.appending(path: Self.releasesName)
        return FileManager.default.fileExists(atPath: url.path) ? url : nil
    }
}

/// How QA asks the app to draw: Symphony's QA launch sets
/// `SYMPHONY_BAR_QA_APPEARANCE` to `light` or `dark`. Read in QA mode only;
/// an unset or unknown value leaves the system's.
public enum QAAppearance: String, Equatable, Sendable {
    case light
    case dark

    public static let variable = "SYMPHONY_BAR_QA_APPEARANCE"

    public static func read(isQAMode: Bool, environment: [String: String]) -> QAAppearance? {
        guard isQAMode, let value = environment[variable] else { return nil }
        return QAAppearance(rawValue: value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased())
    }
}
