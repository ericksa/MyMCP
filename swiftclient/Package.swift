// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "MCPClient",
    platforms: [
        .macOS(.v13),
        .iOS(.v16)
    ],
    products: [
        .library(
            name: "MCPClient",
            targets: ["MCPClient"]
        ),
    ],
    targets: [
        .target(
            name: "MCPClient",
            dependencies: [],
            path: "Sources/MCPClient"
        ),
        .testTarget(
            name: "MCPClientTests",
            dependencies: ["MCPClient"],
            path: "Tests/MCPClientTests"
        ),
    ]
)
