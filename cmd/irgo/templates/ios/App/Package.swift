// swift-tools-version: 6.0
import PackageDescription

// SwiftPM package for the Irgo iOS app, built with xtool
// (https://xtool.sh). Used by `irgo app run ios` on Linux; also buildable
// with `xtool dev` on macOS.
//
// The Irgo binary target is the gomobile-generated framework produced
// by `irgo app build ios` (which copies it here from
// build/ios/Irgo.xcframework).
let package = Package(
    name: "App",
    platforms: [
        .iOS(.v15),
    ],
    products: [
        // An xtool project contains exactly one library product,
        // representing the main app.
        .library(
            name: "App",
            targets: ["App"]
        ),
    ],
    targets: [
        .binaryTarget(
            name: "Irgo",
            path: "Irgo.xcframework"
        ),
        .target(
            name: "App",
            dependencies: ["Irgo"],
            swiftSettings: [
                // The Irgo bridge sources predate Swift 6 strict
                // concurrency; compile them in Swift 5 language mode.
                .swiftLanguageMode(.v5)
            ]
        ),
    ]
)
