// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "JobSearchHub",
    platforms: [.macOS("26.0")],
    targets: [
        .target(name: "JobSearchHubCore"),
        .executableTarget(name: "JobSearchHub", dependencies: ["JobSearchHubCore"]),
        .testTarget(name: "JobSearchHubCoreTests", dependencies: ["JobSearchHubCore"]),
    ]
)
