// CLI tool for creating and managing irgo projects
package main

import (
	"fmt"
	"os"
)

var version = "0.3.1"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "new":
		if len(os.Args) < 3 {
			fmt.Println("Usage: irgo new <project-name>")
			os.Exit(1)
		}
		err = newProject(os.Args[2])

	case "dev":
		err = runDev()

	case "serve":
		err = runServe()

	case "build":
		if len(os.Args) < 3 {
			fmt.Println("Usage: irgo build <ios|android|desktop|all>")
			os.Exit(1)
		}
		target := os.Args[2]
		if target == "desktop" {
			platform := ""
			if len(os.Args) > 3 {
				platform = os.Args[3]
			}
			err = buildDesktop(platform)
		} else {
			err = runBuild(target)
		}

	case "run":
		if len(os.Args) < 3 {
			fmt.Println("Usage: irgo run <ios|android|desktop> [--dev]")
			os.Exit(1)
		}
		platform := os.Args[2]
		devMode := hasFlag(os.Args[3:], "--dev", "-d")

		if platform == "desktop" {
			err = runDesktop(devMode)
		} else {
			err = runMobile(platform, devMode)
		}

	case "templ":
		err = runTempl()

	case "test":
		err = runTest()

	case "install-tools":
		err = installTools()

	case "version", "-v", "--version":
		fmt.Printf("irgo %s\n", version)

	case "help", "-h", "--help":
		if len(os.Args) > 2 {
			printCommandHelp(os.Args[2])
		} else {
			printUsage()
		}

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`irgo - Hypermedia framework for mobile and desktop apps

Usage:
  irgo <command> [arguments]

Commands:
  new <name>       Create a new irgo project
  dev              Run development server with hot reload
  serve            Run server without file watching
  build <target>   Build for mobile/desktop (ios, android, desktop, or all)
  run <platform>   Build and run on simulator or desktop
  templ            Generate templ files
  test             Run tests
  install-tools    Install required dev tools (gomobile, templ, air)
  version          Print version information
  help [command]   Show help for a command

Examples:
  irgo new myapp         Create a new project
  irgo dev               Start dev server with hot reload
  irgo run ios           Build and run on iOS Simulator
  irgo run ios --dev     Hot-reload mode (connects to dev server)
  irgo run android       Build and run on Android Emulator
  irgo run desktop       Run as desktop app
  irgo run desktop --dev Desktop app with devtools enabled
  irgo build ios         Build iOS framework only
  irgo build desktop     Build desktop app for current platform`)
}

func printCommandHelp(cmd string) {
	switch cmd {
	case "new":
		fmt.Println(`irgo new - Create a new irgo project

Usage:
  irgo new <project-name>
  irgo new .              Initialize in current directory

Creates a new project with:
  - main.go           App entry point
  - handlers/         Route handlers
  - templates/        Templ templates
  - static/           CSS and JS assets
  - dev.sh            Development script
  - Makefile          Build targets`)

	case "dev":
		fmt.Println(`irgo dev - Run development server with hot reload

Usage:
  irgo dev

Starts:
  - Air for Go hot reloading
  - Templ file watcher
  - Tailwind CSS watcher (if configured)

Server runs at http://localhost:8080`)

	case "build":
		fmt.Println(`irgo build - Build for mobile and desktop platforms

Usage:
  irgo build ios             Build iOS framework (.xcframework)
  irgo build android         Build Android library (.aar)
  irgo build desktop         Build desktop app for current platform
  irgo build desktop macos   Build desktop app for macOS
  irgo build desktop windows Build desktop app for Windows
  irgo build desktop linux   Build desktop app for Linux
  irgo build all             Build all mobile platforms

Requirements:
  - iOS: Xcode and gomobile
  - Android: Android SDK and gomobile
  - Desktop: CGO enabled (C compiler required)
    - macOS: Xcode Command Line Tools
    - Windows: MinGW-w64 or similar
    - Linux: GCC and WebKit2GTK dev packages

Output:
  - iOS: build/ios/Irgo.xcframework
  - Android: build/android/irgo.aar
  - Desktop macOS: build/desktop/macos/<app>.app
  - Desktop Windows: build/desktop/windows/<app>.exe
  - Desktop Linux: build/desktop/linux/<app>`)

	case "templ":
		fmt.Println(`irgo templ - Generate templ files

Usage:
  irgo templ

Runs 'templ generate' to compile .templ files to Go code.`)

	case "run":
		fmt.Println(`irgo run - Build and run on simulator or desktop

Usage:
  irgo run ios              Build and run on iOS Simulator
  irgo run ios --dev        Run iOS with hot-reload (connects to dev server)
  irgo run android          Build and run on Android Emulator
  irgo run desktop          Run as desktop app
  irgo run desktop --dev    Run desktop app with devtools enabled

Flags:
  --dev, -d    Development mode.
               - Mobile: Connects to localhost:8080 for hot-reload
               - Desktop: Enables browser devtools in webview

Requirements:
  - iOS: Xcode with iOS Simulator
  - Android: Android Studio with emulator
  - Desktop: CGO enabled (see 'irgo help build' for details)

Mobile standard mode (without --dev):
  1. Builds the Go framework with gomobile
  2. Builds the native app project
  3. Installs and launches on simulator/emulator

Mobile dev mode (with --dev):
  1. Starts the dev server (hot-reload)
  2. Builds iOS app without gomobile framework
  3. Launches simulator connected to localhost:8080
  4. Code changes instantly reflect in the app

Desktop mode:
  1. Starts local HTTP server on auto-selected port
  2. Opens native webview window pointing to localhost
  3. Closes server when window is closed`)

	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		printUsage()
	}
}
