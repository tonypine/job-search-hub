import Foundation
@testable import JobSearchHubCore
import Testing

private let ownTeam = "OWNERTEAM1"
private let ownRequirement = #"identifier "com.tonypine.JobSearchHub" and anchor apple generic and certificate leaf[subject.CN] = "Apple Development: Owner (ABCDE12345)""#
private let running = BundleSigning(teamID: ownTeam, designatedRequirement: ownRequirement)
private let runningServer = ServerBuild(version: "0.1.247", newestMigration: 88)

/// Answers the checks' reads for a download, as `codesign` and its
/// `hub-server` would.
private struct FakeInspector: BundleInspecting {
    var refusal: String?
    var signing = BundleSigning(teamID: ownTeam, designatedRequirement: ownRequirement)
    var server: ServerBuild? = ServerBuild(version: "0.1.252", newestMigration: 91)

    func verifySignature(of app: URL) async -> String? { refusal }
    func readSigning(of app: URL) async -> BundleSigning { signing }
    func readServerBuild(of app: URL) async -> ServerBuild? { server }
}

private func makeFolder() throws -> URL {
    let folder = FileManager.default.temporaryDirectory.appending(path: "NewVersionChecksTests-\(UUID().uuidString)")
    try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
    return folder
}

/// A bundle with only its Info.plist, at the version given.
private func makeApp(in folder: URL, version: String = "0.1.252", buildVersion: String? = nil) throws -> URL {
    let app = folder.appending(path: "JobSearchHub.app")
    try FileManager.default.createDirectory(at: app.appending(path: "Contents"), withIntermediateDirectories: true)
    let plist: [String: Any] = ["CFBundleShortVersionString": version, "CFBundleVersion": buildVersion ?? version]
    try PropertyListSerialization.data(fromPropertyList: plist, format: .xml, options: 0)
        .write(to: app.appending(path: "Contents/Info.plist"))
    return app
}

private func check(_ app: URL, inspector: FakeInspector = FakeInspector(), running: BundleSigning = running) async throws(DownloadProblem) -> CheckedVersion {
    try await BundleChecks.check(app, expected: HubVersion("0.1.252")!, running: running, runningServer: runningServer, inspector: inspector)
}

@Test func aDownloadMatchingItsChecksumPasses() throws {
    let folder = try makeFolder()
    let zip = folder.appending(path: "Job-Search-Hub-0.1.252.zip")
    try Data("the app".utf8).write(to: zip)
    let digest = try Checksum.digest(of: zip)

    // As `printf 'the app' | shasum -a 256` prints it.
    #expect(digest == "ecf6410c21e70b6249f59badba275ed326c3043ff2a39f08b88373ead8932dbc")
    #expect(throws: Never.self) { try Checksum.verify(zip, against: "\(digest)  Job-Search-Hub-0.1.252.zip\n") }
}

@Test func theChecksumCheckRefusesAnAlteredDownload() throws {
    let folder = try makeFolder()
    let zip = folder.appending(path: "Job-Search-Hub-0.1.252.zip")
    try Data("the app".utf8).write(to: zip)
    let expected = String(repeating: "a", count: 64)

    #expect(throws: DownloadProblem.checksumMismatch(expected: expected, found: try Checksum.digest(of: zip))) {
        try Checksum.verify(zip, against: "\(expected)  Job-Search-Hub-0.1.252.zip")
    }
    #expect(throws: DownloadProblem.unreadableChecksum) { try Checksum.verify(zip, against: "Not Found") }
}

@Test func aChecksumIsReadAsShasumWritesIt() {
    let digest = String(repeating: "0123456789abcdef", count: 4)
    #expect(Checksum.parse("\(digest)  Job-Search-Hub-0.1.252.zip\n") == digest)
    #expect(Checksum.parse(digest.uppercased()) == digest)
    #expect(Checksum.parse("abc  file") == nil)
    #expect(Checksum.parse("") == nil)
}

@Test func aSignedDownloadOfTheRightVersionPassesAndSaysWhetherItMigrates() async throws {
    let app = try makeApp(in: try makeFolder())

    let checked = try await check(app)

    #expect(checked.version == HubVersion("0.1.252"))
    #expect(checked.changesDatabase == true)

    var sameSchema = FakeInspector()
    sameSchema.server = ServerBuild(version: "0.1.252", newestMigration: 88)
    #expect(try await check(app, inspector: sameSchema).changesDatabase == false)
}

@Test func theSignatureCheckRefusesWhatCodesignRefuses() async throws {
    let app = try makeApp(in: try makeFolder())

    await #expect(throws: DownloadProblem.invalidSignature("a sealed resource is missing or invalid")) {
        try await check(app, inspector: FakeInspector(refusal: "a sealed resource is missing or invalid"))
    }
}

@Test func theTeamCheckRefusesAnotherTeamsSignature() async throws {
    let app = try makeApp(in: try makeFolder())
    var inspector = FakeInspector()
    inspector.signing = BundleSigning(teamID: "OTHERTEAM9", designatedRequirement: ownRequirement)

    await #expect(throws: DownloadProblem.otherTeam(found: "OTHERTEAM9", expected: ownTeam)) {
        try await check(app, inspector: inspector)
    }

    inspector.signing = BundleSigning(teamID: nil, designatedRequirement: nil)
    await #expect(throws: DownloadProblem.otherTeam(found: nil, expected: ownTeam)) {
        try await check(app, inspector: inspector)
    }
}

@Test func theTeamCheckRefusesEverythingWhenTheRunningAppHasNoTeam() async throws {
    let app = try makeApp(in: try makeFolder())

    await #expect(throws: DownloadProblem.runningAppUnsigned) {
        try await check(app, running: BundleSigning(teamID: nil, designatedRequirement: "cdhash H\"0123\""))
    }
}

@Test func theRequirementCheckRefusesAnotherDesignatedRequirement() async throws {
    let app = try makeApp(in: try makeFolder())
    var inspector = FakeInspector()
    inspector.signing = BundleSigning(teamID: ownTeam, designatedRequirement: #"identifier "com.example.Other" and anchor apple generic"#)

    await #expect(throws: DownloadProblem.otherRequirement) {
        try await check(app, inspector: inspector)
    }
}

@Test func theVersionCheckRefusesAnotherVersionInInfoPlist() async throws {
    let app = try makeApp(in: try makeFolder(), version: "0.1.251")

    await #expect(throws: DownloadProblem.otherVersion(place: "Info.plist's CFBundleShortVersionString", found: "0.1.251", expected: "0.1.252")) {
        try await check(app)
    }

    let mixed = try makeApp(in: try makeFolder(), buildVersion: "0.1.250")
    await #expect(throws: DownloadProblem.otherVersion(place: "Info.plist's CFBundleVersion", found: "0.1.250", expected: "0.1.252")) {
        try await check(mixed)
    }
}

@Test func theVersionCheckRefusesAnotherOrAMissingServer() async throws {
    let app = try makeApp(in: try makeFolder())
    var inspector = FakeInspector()
    inspector.server = ServerBuild(version: "0.1.0-dev.abc1234", newestMigration: 91)

    await #expect(throws: DownloadProblem.otherVersion(place: "hub-server --version", found: "0.1.0-dev.abc1234", expected: "0.1.252")) {
        try await check(app, inspector: inspector)
    }

    inspector.server = nil
    await #expect(throws: DownloadProblem.otherVersion(place: "hub-server --version", found: nil, expected: "0.1.252")) {
        try await check(app, inspector: inspector)
    }
}

@Test func readsCodesignsTeamAndRequirement() {
    let details = """
    Executable=/Users/owner/Applications/Job Search Hub.app/Contents/MacOS/JobSearchHub
    Identifier=com.tonypine.JobSearchHub
    Authority=Apple Development: Owner (ABCDE12345)
    TeamIdentifier=OWNERTEAM1
    """
    let requirements = "Executable=/Users/owner/Applications/Job Search Hub.app/Contents/MacOS/JobSearchHub\ndesignated => \(ownRequirement)\n"

    #expect(BundleSigning.parse(details: details, requirements: requirements) == running)
    #expect(BundleSigning.parse(details: "Signature=adhoc\nTeamIdentifier=not set", requirements: "designated => cdhash H\"01\"").teamID == nil)
}

@Test func readsTheServersVersionAndNewestMigration() {
    #expect(ServerBuild.parse("hub-server 0.1.252\ncommit 4c1e8a9f00d1b2c3\nnewest migration 91\n") == ServerBuild(version: "0.1.252", newestMigration: 91))
    #expect(ServerBuild.parse("hub-server 0.1.252\n") == ServerBuild(version: "0.1.252", newestMigration: nil))
    #expect(ServerBuild.parse("usage: hub-server") == nil)
}

@Test func theUpdatesFolderKeepsOneDownloadedVersion() throws {
    let updates = UpdatesFolder(root: try makeFolder())
    let older = HubVersion("0.1.250")!
    let newer = HubVersion("0.1.252")!
    for version in [older, newer] {
        let app = try makeApp(in: try updates.prepareFolder(for: version), version: version.description)
        try updates.recordChecked(CheckedVersion(version: version, app: app, changesDatabase: false), at: .now)
    }
    try FileManager.default.createDirectory(at: updates.previousURL, withIntermediateDirectories: true)
    _ = try makeApp(in: updates.previousURL, version: "0.1.244")
    updates.log("Checked 0.1.252", at: .now)

    updates.removeDownloads(keeping: newer)

    #expect(updates.readChecked(older) == nil)
    #expect(updates.readChecked(newer)?.changesDatabase == false)
    #expect(updates.readPreviousVersion() == HubVersion("0.1.244"))
    #expect(FileManager.default.fileExists(atPath: updates.logURL.path))
}

@Test func aDownloadWithoutItsRecordIsNotOffered() throws {
    let updates = UpdatesFolder(root: try makeFolder())
    let version = HubVersion("0.1.252")!
    _ = try makeApp(in: try updates.prepareFolder(for: version))

    #expect(updates.readChecked(version) == nil)
    #expect(updates.readPreviousVersion() == nil)
}
