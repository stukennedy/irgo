// CLI tool for creating and managing gohtmx projects
package main

import (
	"fmt"
	"os"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "new":
		if len(os.Args) < 3 {
			fmt.Println("Usage: gohtmx new <project-name>")
			os.Exit(1)
		}
		err = newProject(os.Args[2])

	case "dev":
		err = runDev()

	case "serve":
		err = runServe()

	case "build":
		if len(os.Args) < 3 {
			fmt.Println("Usage: gohtmx build <ios|android|desktop|all>")
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
			fmt.Println("Usage: gohtmx run <ios|android|desktop> [--dev]")
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
		fmt.Printf("gohtmx %s\n", version)

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
	fmt.Println(`gohtmx - Hypermedia framework for mobile and desktop apps

Usage:
  gohtmx <command> [arguments]

Commands:
  new <name>       Create a new gohtmx project
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
  gohtmx new myapp         Create a new project
  gohtmx dev               Start dev server with hot reload
  gohtmx run ios           Build and run on iOS Simulator
  gohtmx run ios --dev     Hot-reload mode (connects to dev server)
  gohtmx run android       Build and run on Android Emulator
  gohtmx run desktop       Run as desktop app
  gohtmx run desktop --dev Desktop app with devtools enabled
  gohtmx build ios         Build iOS framework only
  gohtmx build desktop     Build desktop app for current platform`)
}

func printCommandHelp(cmd string) {
	switch cmd {
	case "new":
		fmt.Println(`gohtmx new - Create a new gohtmx project

Usage:
  gohtmx new <project-name>
  gohtmx new .              Initialize in current directory

Creates a new project with:
  - main.go           App entry point
  - handlers/         Route handlers
  - templates/        Templ templates
  - static/           CSS and JS assets
  - dev.sh            Development script
  - Makefile          Build targets`)

	case "dev":
		fmt.Println(`gohtmx dev - Run development server with hot reload

Usage:
  gohtmx dev

Starts:
  - Air for Go hot reloading
  - Templ file watcher
  - Tailwind CSS watcher (if configured)

Server runs at http://localhost:8080`)

	case "build":
		fmt.Println(`gohtmx build - Build for mobile and desktop platforms

Usage:
  gohtmx build ios             Build iOS framework (.xcframework)
  gohtmx build android         Build Android library (.aar)
  gohtmx build desktop         Build desktop app for current platform
  gohtmx build desktop macos   Build desktop app for macOS
  gohtmx build desktop windows Build desktop app for Windows
  gohtmx build desktop linux   Build desktop app for Linux
  gohtmx build all             Build all mobile platforms

Requirements:
  - iOS: Xcode and gomobile
  - Android: Android SDK and gomobile
  - Desktop: CGO enabled (C compiler required)
    - macOS: Xcode Command Line Tools
    - Windows: MinGW-w64 or similar
    - Linux: GCC and WebKit2GTK dev packages

Output:
  - iOS: build/ios/Gohtmx.xcframework
  - Android: build/android/gohtmx.aar
  - Desktop macOS: build/desktop/macos/<app>.app
  - Desktop Windows: build/desktop/windows/<app>.exe
  - Desktop Linux: build/desktop/linux/<app>`)

	case "templ":
		fmt.Println(`gohtmx templ - Generate templ files

Usage:
  gohtmx templ

Runs 'templ generate' to compile .templ files to Go code.`)

	case "run":
		fmt.Println(`gohtmx run - Build and run on simulator or desktop

Usage:
  gohtmx run ios              Build and run on iOS Simulator
  gohtmx run ios --dev        Run iOS with hot-reload (connects to dev server)
  gohtmx run android          Build and run on Android Emulator
  gohtmx run desktop          Run as desktop app
  gohtmx run desktop --dev    Run desktop app with devtools enabled

Flags:
  --dev, -d    Development mode.
               - Mobile: Connects to localhost:8080 for hot-reload
               - Desktop: Enables browser devtools in webview

Requirements:
  - iOS: Xcode with iOS Simulator
  - Android: Android Studio with emulator
  - Desktop: CGO enabled (see 'gohtmx help build' for details)

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
