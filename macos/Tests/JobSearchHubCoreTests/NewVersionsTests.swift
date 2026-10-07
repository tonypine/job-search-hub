import Foundation
import HubTestSupport
@testable import JobSearchHubCore
import Testing

// Made-up changes only, in scripts/release/changelog.sh's format.

private func version(_ text: String) -> HubVersion {
    HubVersion(text)!
}

private func makeRelease(_ tag: String, draft: Bool = false, prerelease: Bool = false, body: String = "") -> GitHubRelease {
    let name = tag.replacingOccurrences(of: "mac-v", with: "Job-Search-Hub-")
    return GitHubRelease(
        tagName: tag, draft: draft, prerelease: prerelease, body: body,
        htmlURL: URL(string: "https://github.com/\(GitHubRepository.slug)/releases/tag/\(tag)"),
        assets: [
            .init(name: "\(name).zip", browserDownloadURL: URL(string: "https://example.com/\(name).zip")!),
            .init(name: "\(name).zip.sha256", browserDownloadURL: URL(string: "https://example.com/\(name).zip.sha256")!),
        ]
    )
}

@Test func versionsOrderByTheirCommitCount() {
    #expect(version("0.1.9") < version("0.1.10"))
    #expect(version("0.1.252") < version("0.2.0"))
    #expect(version("0.1.252") == version("0.1.252"))
    #expect(HubVersion("0.1") == nil)
    #expect(HubVersion("mac-v0.1.2") == nil)
    #expect(HubVersion(" 0.1.3\n")?.description == "0.1.3")
}

@Test func aLocalBuildIsKnownByItsVersionAndSortsBelowReleases() {
    let local = version("0.1.0-dev.abc1234")
    #expect(local.isLocalBuild)
    #expect(local.localCommit == "abc1234")
    #expect(local < version("0.1.1"))
    #expect(version("0.1.252-dev.abc1234") < version("0.1.252"))
    #expect(version("0.1.0-dev").localCommit == nil)
    #expect(!version("0.1.252").isLocalBuild)
}

@Test func decodesGitHubsReleaseList() throws {
    let json = """
    [{"tag_name": "mac-v0.1.252", "name": "Job Search Hub 0.1.252 for Mac", "draft": false, "prerelease": false,
      "body": "## New\\n\\n- Mac: Add a thing (#3)\\n", "html_url": "https://github.com/o/r/releases/tag/mac-v0.1.252",
      "published_at": "2026-10-05T14:02:11Z",
      "assets": [{"name": "Job-Search-Hub-0.1.252.zip", "browser_download_url": "https://github.com/o/r/releases/download/mac-v0.1.252/Job-Search-Hub-0.1.252.zip", "size": 31457280}]}]
    """
    let releases = try GitHubRelease.decodeList(Data(json.utf8))

    #expect(releases.count == 1)
    #expect(releases[0].tagName == "mac-v0.1.252")
    #expect(releases[0].htmlURL?.absoluteString == "https://github.com/o/r/releases/tag/mac-v0.1.252")
    #expect(releases[0].assets[0].browserDownloadURL.lastPathComponent == "Job-Search-Hub-0.1.252.zip")
    #expect(releases[0].publishedAt != nil)
}

@Test func picksTheNewestMacReleaseAboveTheRunningOne() {
    let releases = [
        makeRelease("android-v0.1.260"),
        makeRelease("mac-v0.1.250"),
        makeRelease("mac-v0.1.252"),
        makeRelease("mac-v0.1.247"),
        makeRelease("mac-vnext"),
    ]

    #expect(MacReleases.findNewest(in: releases, above: version("0.1.247"))?.version == version("0.1.252"))
    #expect(MacReleases.findNewest(in: releases, above: version("0.1.252")) == nil)
    #expect(MacReleases.findNewest(in: releases, above: version("0.1.0-dev.abc1234"))?.version == version("0.1.252"))
    #expect(MacReleases.findNewest(in: releases, above: version("0.1.247"))?.zipAsset?.name == "Job-Search-Hub-0.1.252.zip")
    #expect(MacReleases.findNewest(in: releases, above: version("0.1.247"))?.checksumAsset?.name == "Job-Search-Hub-0.1.252.zip.sha256")
}

@Test func skipsDraftsAndPreReleases() {
    let releases = [
        makeRelease("mac-v0.1.255", draft: true),
        makeRelease("mac-v0.1.254", prerelease: true),
        makeRelease("mac-v0.1.252"),
    ]

    #expect(MacReleases.findNewest(in: releases, above: version("0.1.247"))?.version == version("0.1.252"))
    #expect(MacReleases.getOffered(releases).map(\.version) == [version("0.1.252")])
    #expect(MacReleases.findNewest(in: [makeRelease("mac-v0.1.254", prerelease: true)], above: version("0.1.247")) == nil)
}

@Test func aRunningVersionIsWithdrawnOnceItsReleaseIsAPreRelease() {
    let releases = [makeRelease("mac-v0.1.252", prerelease: true), makeRelease("mac-v0.1.247")]

    #expect(MacReleases.isWithdrawn(version("0.1.252"), in: releases))
    #expect(!MacReleases.isWithdrawn(version("0.1.247"), in: releases))
    #expect(!MacReleases.isWithdrawn(version("0.1.0-dev.abc1234"), in: releases))
    #expect(!MacReleases.isWithdrawn(version("0.1.252"), in: [makeRelease("mac-v0.1.252", draft: true, prerelease: true)]))
}

@Test func theReleasesInBetweenRunFromAfterTheRunningOneToTheNewest() {
    let releases = ["mac-v0.1.244", "mac-v0.1.247", "mac-v0.1.249", "mac-v0.1.250", "mac-v0.1.252"].map { makeRelease($0) }
        + [makeRelease("mac-v0.1.251", prerelease: true)]

    let between = MacReleases.findBetween(in: releases, running: version("0.1.247"), through: version("0.1.250"))

    #expect(between.map(\.version) == [version("0.1.250"), version("0.1.249")])
}

@Test func parsesTheChangelogsSectionsTagsAndPullRequests() {
    let notes = ReleaseNotes.parse("""
    ## New

    - Mac: Suggest a second route for unanswered applications (#67)
    - Server, Phone: Send a phone the new jobs (#9)

    ## Fixed

    - Mac, Server: Keep the inspector's width (#8)
    - Mac: Took out: suggest a second route (#7) (#12)

    ## Other changes

    - Server: chore: build the server on Go 1.27 (#13)
    - Phone: Update the README
    - chore: pin staticcheck in CI (#72)
    """)

    #expect(notes == [
        ChangeNote(kind: .new, parts: [.mac], text: "Suggest a second route for unanswered applications", pullRequest: 67),
        ChangeNote(kind: .new, parts: [.server, .phone], text: "Send a phone the new jobs", pullRequest: 9),
        ChangeNote(kind: .fixed, parts: [.mac, .server], text: "Keep the inspector's width", pullRequest: 8),
        ChangeNote(kind: .fixed, parts: [.mac], text: "Took out: suggest a second route (#7)", pullRequest: 12),
        ChangeNote(kind: .other, parts: [.server], text: "chore: build the server on Go 1.27", pullRequest: 13),
        ChangeNote(kind: .other, parts: [.phone], text: "Update the README"),
        ChangeNote(kind: .other, parts: [], text: "chore: pin staticcheck in CI", pullRequest: 72),
    ])
    #expect(notes[1].partsLabel == "Server, Phone")
    #expect(notes[0].pullRequestURL?.absoluteString == "https://github.com/tonypine/job-search-hub/pull/67")
}

@Test func aChangelogWithNothingInItHasNoNotes() {
    #expect(ReleaseNotes.parse("No changes under server/ macos/ since mac-v0.1.250.").isEmpty)
    #expect(ReleaseNotes.parse("").isEmpty)
    // A list item outside the known sections isn't a change.
    #expect(ReleaseNotes.parse("## Contributors\n\n- someone").isEmpty)
}

@Test func mergesSeveralReleasesNotesNewestFirstGroupedByKind() {
    let releases = [
        makeRelease("mac-v0.1.249", body: "## New\n\n- Mac: Suggest a second route (#67)\n\n## Other changes\n\n- Server: chore: pin staticcheck (#68)\n"),
        makeRelease("mac-v0.1.252", body: "## Fixed\n\n- Server: Retry board polls that time out (#71)\n\n## Other changes\n\n- Mac: refactor: split the page (#72)\n"),
        makeRelease("mac-v0.1.250", body: "## New\n\n- Phone: Snooze a follow-up (#73)\n\n## Fixed\n\n- Mac: Keep the inspector from looping (#66)\n"),
    ]
    let macReleases = MacReleases.getOffered(releases)

    let whatsNew = WhatsNew.merge(macReleases)

    #expect(whatsNew.releaseCount == 3)
    #expect(whatsNew.notes.map(\.pullRequest) == [73, 67, 71, 66, 72, 68])
    #expect(whatsNew.highlights.map(\.pullRequest) == [73, 67, 71, 66])
    #expect(whatsNew.getNotes(.other).count == 2)
    #expect(whatsNew.summary == "3 releases · 6 changes")
    #expect(whatsNew.hasPhoneChanges)
}

@Test func aChangeTwoReleasesListShowsOnce() {
    let line = "## Fixed\n\n- Mac: Keep the inspector from looping (#66)\n"
    let whatsNew = WhatsNew.merge(MacReleases.getOffered([makeRelease("mac-v0.1.250", body: line), makeRelease("mac-v0.1.251", body: line)]))

    #expect(whatsNew.changeCount == 1)
    #expect(whatsNew.summary == "2 releases · 1 change")
    #expect(!whatsNew.hasPhoneChanges)
}

@Test func theFeedAsksGitHubWithoutATokenAndReadsTheReleases() async throws {
    let body = #"[{"tag_name": "mac-v0.1.252", "draft": false, "prerelease": false, "assets": []}]"#
    let (session, recording) = StubHub.makeSession(answers: ["/repos/tonypine/job-search-hub/releases": .init(status: 200, body: body)])

    let releases = try await ReleaseFeed(session: session, appVersion: "0.1.247").fetch()

    #expect(releases.map(\.tagName) == ["mac-v0.1.252"])
    #expect(recording.lastRequest?.url?.host() == "api.github.com")
    #expect(recording.lastRequest?.value(forHTTPHeaderField: "Authorization") == nil)
    #expect(recording.lastRequest?.value(forHTTPHeaderField: "User-Agent") == "JobSearchHub/0.1.247")
}

@Test func theFeedSaysWhenGitHubTurnsItAway() async {
    let (session, _) = StubHub.makeSession(answers: ["/repos/tonypine/job-search-hub/releases": .init(status: 403, body: "{}")])

    await #expect(throws: ReleaseFeed.Failure.rateLimited) {
        try await ReleaseFeed(session: session).fetch()
    }
}

@Test func theFeedReadsHUB_RELEASES_URLOnlyInQAMode() {
    let environment = [ReleaseFeed.urlVariable: "http://localhost:8765/releases.json"]

    #expect(ReleaseFeed.makeURL(arguments: ["JobSearchHub", "--qa-mode"], environment: environment) == URL(string: "http://localhost:8765/releases.json"))
    #expect(ReleaseFeed.makeURL(arguments: ["JobSearchHub"], environment: environment) == GitHubRepository.releasesAPIURL)
    #expect(ReleaseFeed.makeURL(arguments: ["JobSearchHub", "--qa-mode"], environment: [:]) == GitHubRepository.releasesAPIURL)
    // A QA build is in QA mode without the flag.
    #expect(ReleaseFeed.makeURL(arguments: ["JobSearchHub"], environment: environment, isQABuild: true) == URL(string: "http://localhost:8765/releases.json"))
}

@Test func aHUB_RELEASES_URLThatIsntAnHTTPURLLeavesTheFeedOnGitHub() {
    for value in ["", "localhost:8765/releases.json", "file:///tmp/releases.json", "not a url"] {
        let url = ReleaseFeed.makeURL(arguments: ["JobSearchHub", "--qa-mode"], environment: [ReleaseFeed.urlVariable: value])
        #expect(url == GitHubRepository.releasesAPIURL, "\(value)")
    }
}
