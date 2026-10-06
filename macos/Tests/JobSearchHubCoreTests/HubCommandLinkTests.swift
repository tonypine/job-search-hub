import Foundation
@testable import JobSearchHubCore
import Testing

private func makeHome() throws -> URL {
    let home = FileManager.default.temporaryDirectory.appending(path: "hub-command-link-\(UUID().uuidString)")
    try FileManager.default.createDirectory(at: home, withIntermediateDirectories: true)
    return home
}

@Test func installingTheHubCommandLinksItToTheBundlesCopy() throws {
    let home = try makeHome()
    defer { try? FileManager.default.removeItem(at: home) }
    let link = HubCommandLink.makeLinkURL(home: home)
    let target = ServerLaunchAgent.makeCommandURL("hub", bundle: home.appending(path: "Applications/Job Search Hub.app"))
    #expect(link.path == home.path + "/.local/bin/hub")
    #expect(HubCommandLink.readState(link: link, target: target) == .missing)

    try HubCommandLink.install(link: link, target: target)
    #expect(HubCommandLink.readState(link: link, target: target) == .linked)
    #expect(try FileManager.default.destinationOfSymbolicLink(atPath: link.path) == target.path)
}

@Test func installingReplacesAnotherHub() throws {
    let home = try makeHome()
    defer { try? FileManager.default.removeItem(at: home) }
    let link = HubCommandLink.makeLinkURL(home: home)
    let target = home.appending(path: "Applications/Job Search Hub.app/Contents/Helpers/bin/hub")
    try FileManager.default.createDirectory(at: link.deletingLastPathComponent(), withIntermediateDirectories: true)

    try FileManager.default.createSymbolicLink(atPath: link.path, withDestinationPath: "/opt/old/hub")
    #expect(HubCommandLink.readState(link: link, target: target) == .other(destination: "/opt/old/hub"))
    try HubCommandLink.install(link: link, target: target)
    #expect(HubCommandLink.readState(link: link, target: target) == .linked)

    try FileManager.default.removeItem(at: link)
    try Data("#!/bin/sh\n".utf8).write(to: link)
    #expect(HubCommandLink.readState(link: link, target: target) == .other(destination: nil))
    try HubCommandLink.install(link: link, target: target)
    #expect(HubCommandLink.readState(link: link, target: target) == .linked)
}
