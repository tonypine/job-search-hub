// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "JobSearchHub",
    platforms: [.macOS("26.0")],
    dependencies: [
        .package(url: "https://github.com/migueldeicaza/SwiftTerm.git", from: "1.2.0"),
    ],
    targets: [
        .target(name: "JobSearchHubCore"),
        .executableTarget(
            name: "JobSearchHub",
            dependencies: ["JobSearchHubCore", .product(name: "SwiftTerm", package: "SwiftTerm")]
        ),
        .testTarget(name: "JobSearchHubCoreTests", dependencies: ["JobSearchHubCore"]),
    ]
)
