// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "JobSearchHub",
    platforms: [.macOS("26.0")],
    products: [
        .executable(name: "JobSearchHub", targets: ["JobSearchHub"]),
        .executable(name: "hub-cvprint", targets: ["CVPrint"]),
    ],
    dependencies: [
        .package(url: "https://github.com/migueldeicaza/SwiftTerm.git", from: "1.2.0"),
    ],
    targets: [
        .target(name: "JobSearchHubCore"),
        .executableTarget(
            name: "JobSearchHub",
            dependencies: ["JobSearchHubCore", .product(name: "SwiftTerm", package: "SwiftTerm")]
        ),
        // The hub server prints CVs with it: WebKit, offscreen, no window.
        .executableTarget(name: "CVPrint"),
        // What both test targets share: StubHub, the canned URLSession.
        .target(name: "HubTestSupport", path: "Tests/HubTestSupport"),
        .testTarget(name: "JobSearchHubCoreTests", dependencies: ["JobSearchHubCore", "HubTestSupport"]),
        // The app's views, drawn offscreen with ImageRenderer, and the app's
        // new-version checks.
        .testTarget(name: "JobSearchHubTests", dependencies: ["JobSearchHub", "JobSearchHubCore", "HubTestSupport"]),
    ]
)
