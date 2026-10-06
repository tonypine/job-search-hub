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
        .testTarget(name: "JobSearchHubCoreTests", dependencies: ["JobSearchHubCore"]),
        // The app's views, drawn offscreen with ImageRenderer.
        .testTarget(name: "JobSearchHubTests", dependencies: ["JobSearchHub", "JobSearchHubCore"]),
    ]
)
