package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ---------------------------------------------------------------------------
// package configuration (irgo.package.toml)
//
// `irgo project new` scaffolds a default irgo.package.toml at the project root. The
// `irgo app package` commands read it for defaults; explicit CLI flags override
// the file. Missing file = built-in defaults + CLI flags.
// ---------------------------------------------------------------------------

type packageConfig struct {
	Version                  string // common.version (android versionName, windows version)
	Icon                     string // common.icon (single source icon for all stores)
	IOSTeam                  string // ios.team
	IOSExportMethod          string // ios.export_method
	AndroidKeystore          string // android.keystore
	AndroidKeystorePw        string // android.keystore_pass
	AndroidKeyAlias          string // android.key_alias
	AndroidKeyPass           string // android.key_pass
	WindowsPublisher         string // windows.publisher
	WindowsCert              string // windows.cert
	WindowsCertPass          string // windows.cert_pass
	WindowsIcon              string // windows.icon
	MacIdentity              string // macos.identity
	MacNotarize              bool   // macos.notarize
	MacAppleID               string // macos.apple_id
	MacTeam                  string // macos.team
	MacPassword              string // macos.password
	MacDMG                   bool   // macos.dmg
	ReviewsIOSAppID          string // reviews.ios_app_id (numeric iOS App Store id)
	ReviewsMacAppID          string // reviews.mac_app_id (numeric Mac App Store id)
	ReviewsIOSKeyID          string // reviews.ios_key_id (App Store Connect API key id)
	ReviewsIOSIssuerID       string // reviews.ios_issuer_id (ASC API issuer id)
	ReviewsIOSPrivateKey     string // reviews.ios_private_key (.p8 path)
	ReviewsAndroidPackage    string // reviews.android_package (Play package name)
	ReviewsAndroidServiceAcc string // reviews.android_service_account (JSON path)
}

const (
	packageConfigFile = "irgo.package.toml"
	// packageLocalFile holds machine-local secrets. It is gitignored and
	// overrides the shared file, so passwords and certificate paths never have
	// to be committed to share the rest of the configuration.
	packageLocalFile = "irgo.package.local.toml"
)

// parsePackageConfig reads the simple, fixed-format irgo.package.toml:
//
//	[common]
//	version = "0.1.0"
//	[ios]
//	team = ""
//	...
//
// Returns an empty config when the file is absent or unreadable (callers fall
// back to built-in defaults + flags). Only `key = "value"` (with # comments
// and [section] headers) is supported — this is our own file format.
func parsePackageConfig() packageConfig {
	cfg := parsePackageConfigFile(packageConfigFile, packageConfig{})
	// The local overlay wins: it is where secrets live.
	return parsePackageConfigFile(packageLocalFile, cfg)
}

// parsePackageConfigFile layers one file over an existing config. Keys absent
// from the file leave the previous value untouched, so the local overlay only
// has to name what it changes.
func parsePackageConfigFile(path string, cfg packageConfig) packageConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	section := ""
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.Trim(strings.TrimSpace(line[eq+1:]), `"'`)
		switch section {
		case "common":
			switch key {
			case "version":
				cfg.Version = val
			case "icon":
				cfg.Icon = val
			}
		case "ios":
			switch key {
			case "team":
				cfg.IOSTeam = val
			case "export_method":
				cfg.IOSExportMethod = val
			}
		case "android":
			switch key {
			case "keystore":
				cfg.AndroidKeystore = val
			case "keystore_pass":
				cfg.AndroidKeystorePw = val
			case "key_alias":
				cfg.AndroidKeyAlias = val
			case "key_pass":
				cfg.AndroidKeyPass = val
			}
		case "windows":
			switch key {
			case "publisher":
				cfg.WindowsPublisher = val
			case "cert":
				cfg.WindowsCert = val
			case "cert_pass":
				cfg.WindowsCertPass = val
			case "icon":
				cfg.WindowsIcon = val
			}
		case "macos":
			switch key {
			case "identity":
				cfg.MacIdentity = val
			case "notarize":
				cfg.MacNotarize = val == "true" || val == "1" || val == "yes"
			case "apple_id":
				cfg.MacAppleID = val
			case "team":
				cfg.MacTeam = val
			case "password":
				cfg.MacPassword = val
			case "dmg":
				cfg.MacDMG = val == "true" || val == "1" || val == "yes"
			}
		case "reviews":
			switch key {
			case "ios_app_id":
				cfg.ReviewsIOSAppID = val
			case "mac_app_id":
				cfg.ReviewsMacAppID = val
			case "ios_key_id":
				cfg.ReviewsIOSKeyID = val
			case "ios_issuer_id":
				cfg.ReviewsIOSIssuerID = val
			case "ios_private_key":
				cfg.ReviewsIOSPrivateKey = val
			case "android_package":
				cfg.ReviewsAndroidPackage = val
			case "android_service_account":
				cfg.ReviewsAndroidServiceAcc = val
			}
		}
	}
	return cfg
}

// writeDefaultPackageConfig scaffolds irgo.package.toml with documented
// defaults (missing-only, like the rest of the CLI).
func writeDefaultPackageConfig() error {
	if _, err := os.Stat(packageConfigFile); err == nil {
		return nil
	}
	content := `# irgo app package configuration — defaults for ` + "`irgo app package <ios|android|macos|windows>`" + `
#
# CLI flags override any value here (e.g. ` + "`irgo app package android --keystore ...`" + `).
# Run ` + "`irgo app package setup`" + ` for a step-by-step guide to obtaining every value
# below for each store.

[common]
version = "0.1.0"

[ios]
# Apple Team ID — 10-character ID. Get it: developer.apple.com → Membership →
# "Team ID". Requires a paid Apple Developer account (US$99/year). Required
# for automatic signing.
team = ""
# app-store (default) | ad-hoc | development
export_method = "app-store"

[android]
# Debug keystore works out of the box (validation only). For the Play Store
# create a release keystore and KEEP IT SAFE — you can't change it after your
# first upload:
#   keytool -genkey -v -keystore ~/keys/release.keystore -alias myapp \
#     -keyalg RSA -keysize 2048 -validity 10000
keystore = "~/.android/debug.keystore"
keystore_pass = "android"
# Empty = auto-detected from the keystore.
key_alias = ""
key_pass = "android"

[windows]
# Publisher DN — must match your Partner Center publisher identity. Get it:
# Partner Center → your app → Product management → Product identity →
# "Publisher display name"/Publisher ID (partner.microsoft.com). A Microsoft
# Store developer account costs US$19 one-time. Empty = test-signed with
# "CN=<app>".
publisher = ""
# Optional real code-signing cert (PFX) + password; empty = self-signed test cert.
cert = ""
cert_pass = ""
icon = ""

[macos]
# Developer ID Application certificate name, e.g.
# "Developer ID Application: Your Name (TEAMID)". Get it:
# developer.apple.com → Certificates → create a Developer ID Application cert →
# download + double-click to import into Keychain. Requires a paid account.
identity = ""
# Notarization: Apple ID + Team ID + an app-specific password
# (appleid.apple.com → Sign-In & Security → App-Specific Passwords).
notarize = false
apple_id = ""
team = ""
password = ""
# Also produce a .dmg
dmg = false

[reviews]
# App Store review monitoring (` + "`irgo app reviews <ios|mac|android>`" + `).
# Apple (iOS + Mac) uses the official App Store Connect API — a key with
# Customer Reviews access. Android uses the Play Developer API (service
# account) and can also reply from the terminal.
# iOS/Mac: numeric App Store ids. Get them: the app's apps.apple.com URL
# (apps.apple.com/app/id<THIS NUMBER>) — iOS and Mac ids differ.
ios_app_id = ""
mac_app_id = ""
# Apple: App Store Connect API key. Get it: App Store Connect → Users and
# Access → Integrations → API keys → generate a key with access to Customer
# Reviews, then download the .p8 (it can only be downloaded once).
ios_key_id = ""
ios_issuer_id = ""
ios_private_key = ""
# Android: Play package name, e.g. "com.example.myapp".
android_package = ""
# Android: path to a Google Play Developer API service-account JSON. Get it:
# Google Play Console → Setup → API access → create a service account →
# grant "View app information" (read) or "Reply to reviews" (write) →
# download the JSON key.
android_service_account = ""
`
	if err := os.WriteFile(packageConfigFile, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Printf("  created: %s (edit for packaging defaults)\n", packageConfigFile)
	return nil
}

// ---------------------------------------------------------------------------
// OS gating — a packaging target only runs on an OS that supports its tools.
// ---------------------------------------------------------------------------

// preparePackage gates the target against the host and regenerates the
// gitignored-but-embedded assets.
//
// Both matter before packaging. Without the regeneration a package built after
// `irgo project clean` embeds no stylesheet and ships an unstyled app — the failure
// does not surface until someone opens the artifact, which for a store build
// is far too late.
func preparePackage(target string) error {
	if err := gateOS(target); err != nil {
		return err
	}
	return ensureAssets()
}

func gateOS(target string) error {
	switch target {
	case "ios":
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("irgo app package ios requires macOS (Xcode + codesign); this host is %s — run it on a Mac or in CI on macos-latest", runtime.GOOS)
		}
	case "windows":
		if runtime.GOOS != "windows" {
			return fmt.Errorf("irgo app package windows requires Windows (MakeAppx/signtool from the Windows SDK); this host is %s — run it on Windows or in CI on windows-latest", runtime.GOOS)
		}
	case "macos":
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("irgo app package macos requires macOS (Xcode codesign/notarytool/hdiutil); this host is %s — run it on a Mac or in CI on macos-latest", runtime.GOOS)
		}
	case "android":
		// Cross-platform: gradle + JDK + Android SDK run on every OS.
	}
	return nil
}

// packageSetupGuide prints where to get every store config value — run with
// `irgo app package setup` and fill the results into irgo.package.toml.
func packageSetupGuide() {
	fmt.Print(`irgo app package — what each store needs, and where to get it

Everything below goes into irgo.package.toml (created by ` + "`irgo app package`" + ` or
` + "`irgo project new`" + `). CLI flags override it per invocation. One source icon
(appicon.png or [common] icon) feeds every store.

COMMON
  version ..... your app version, e.g. "1.2.3" (android versionName, MSIX version)
  icon ........ one PNG, e.g. "appicon.png" (also picked up from static/icon.png)

iOS — App Store (.ipa)
  Account ..... Apple Developer Program, US$99/year: developer.apple.com
  team ........ 10-char "Team ID": developer.apple.com → Membership → Team ID
  Automatic signing handles certificates + provisioning via Xcode.

Android — Google Play (.aab)
  Account ..... Google Play Console, US$25 one-time: play.google.com/console
  Keystore .... Debug keystore works for validation. For release:
                keytool -genkey -v -keystore ~/keys/release.keystore \
                  -alias myapp -keyalg RSA -keysize 2048 -validity 10000
                KEEP IT SAFE — it cannot be changed after your first upload.
  key_alias ... auto-detected from the keystore if left empty.

Windows — Microsoft Store (.msix)
  Account ..... Partner Center, US$19 one-time: partner.microsoft.com
  publisher ... Partner Center → your app → Product identity →
                "Publisher display name"/Publisher ID (put the CN=... form
                in [windows] publisher). Empty = test-signed "CN=<app>".
  cert ........ optional real code-signing cert (PFX); empty = self-signed
                test cert. The Store re-signs on upload.

macOS — notarized distribution (.app/.dmg)
  Account ..... Apple Developer Program (same as iOS).
  identity .... "Developer ID Application: Your Name (TEAMID)":
                developer.apple.com → Certificates → Developer ID Application →
                download + double-click to import into Keychain.
  notarize .... true + apple_id (your Apple ID), team (same Team ID),
                password = app-specific password (appleid.apple.com →
                Sign-In & Security → App-Specific Passwords).

TIPS
  - ` + "`irgo app package ios --team X`" + ` etc. overrides the config file for one run.
  - Every value has an IRGO_* env var (e.g. IRGO_IOS_TEAM), so CI supplies
    secrets as env vars / GitHub secrets — irgo.package.toml is gitignored.
  - ` + "`irgo app package setup <store>`" + ` interactively walks you through that store's
    required values (opens the right pages); ` + "`irgo app package setup --check`" + ` shows
    what's set/missing for every store. In CI (no terminal) commands fail fast
    with the exact env var + URL for each missing value.
  - iOS/macOS packaging must run on macOS; MSIX must run on Windows;
    Android runs anywhere. The CLI enforces this per target.
  - Each target outputs to its own dist/<target>/ folder, never touching
    the unpackaged build/ trees.
`)
}

// projectBaseName returns the current project's directory name (for default
// artifact/display names).
func projectBaseName() string {
	if modulePath, err := getModulePath(); err == nil {
		if b := filepath.Base(modulePath); b != "." && b != "" && b != "/" {
			return b
		}
	}
	wd, _ := os.Getwd()
	if b := filepath.Base(wd); b != "" && b != "/" {
		return b
	}
	return "irgo"
}

// distPath returns the dedicated packaging output dir for a target, keeping
// packaged artifacts separate from the unpackaged build/ trees.
func distPath(target string) string {
	return filepath.Join("dist", target)
}

// ---------------------------------------------------------------------------
// shared package helpers
// ---------------------------------------------------------------------------

// expandHome expands a leading "~" or "~/".
func expandHome(p string) string {
	if p == "~" {
		return homeDir()
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(homeDir(), p[2:])
	}
	return p
}

// copyTree recursively copies a directory tree.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}

// packageCommand parses the flags for `irgo app package` and dispatches to the
// per-store packager. Parsing lives here so the router stays a table of what
// exists rather than a place flags accumulate.
func packageCommand(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: irgo app package <ios|android|macos|windows> [flags]")
	}
	target := args[0]
	team, exportMethod, out := "", "app-store", ""
	keystore, keystorePass, keyAlias, keyPass := "", "", "", ""
	version, publisher, icon, cert, certPass := "", "", "", "", ""
	identity, appleID, password := "", "", ""
	notarize, dmg := false, false
	// setup has its own args (store / --check) — don't parse package flags.
	if target != "setup" {
		for i := 1; i < len(args); i++ {
			next := func() string {
				if i+1 < len(args) {
					i++
					return args[i]
				}
				return ""
			}
			switch args[i] {
			case "--team":
				team = next()
			case "--export-method":
				exportMethod = next()
			case "--keystore":
				keystore = next()
			case "--keystore-pass":
				keystorePass = next()
			case "--key-alias":
				keyAlias = next()
			case "--key-pass":
				keyPass = next()
			case "--version":
				version = next()
			case "--publisher":
				publisher = next()
			case "--icon":
				icon = next()
			case "--cert":
				cert = next()
			case "--cert-pass":
				certPass = next()
			case "--identity":
				identity = next()
			case "--apple-id":
				appleID = next()
			case "--password":
				password = next()
			case "--notarize":
				notarize = true
			case "--dmg":
				dmg = true
			case "-o", "--output":
				out = next()
			default:
				return fmt.Errorf("unknown flag: %s", args[i])
			}
		}
	}
	switch target {
	case "setup":
		// irgo app package setup              → static guide (where to get every value)
		// irgo app package setup <store>      → interactive wizard for that store
		// irgo app package setup --check      → status report for every store
		// irgo app package setup --check <s>  → status report for one store
		check := false
		store := ""
		for _, a := range args[1:] {
			switch a {
			case "--check", "-c":
				check = true
			default:
				store = a
			}
		}
		if check {
			stores := []string{"ios", "android", "windows", "macos", "reviews-apple-ios", "reviews-apple-mac", "reviews-android"}
			if store != "" {
				stores = []string{store}
			}
			for _, s := range stores {
				checkStoreConfig(s)
			}
		} else if store != "" {
			if storeConfigValues(store) == nil {
				fmt.Printf("unknown setup target: %s (use ios, android, windows, macos, or reviews-*)\n", store)
				os.Exit(1)
			}
			missing := missingStoreConfig(store)
			if len(missing) == 0 {
				fmt.Printf("Nothing missing for %s — defaults cover it. Run `irgo app package setup` for the full guide.\n", store)
			} else if wErr := runSetupWizard(store, missing); wErr != nil {
				return wErr
			}
		} else {
			packageSetupGuide()
		}
	case "ios":
		return packageIOS(team, exportMethod, out)
	case "android":
		return packageAndroid(keystore, keystorePass, keyAlias, keyPass, version, icon, out)
	case "macos":
		return packageMacOS(identity, notarize, appleID, team, password, dmg, icon, out)
	case "windows":
		return packageWindows(publisher, version, icon, cert, certPass, out)
	default:
		return fmt.Errorf("unknown package target: %s (use ios, android, macos, windows, or setup)", target)
	}
	return nil
}

func init() {
	register(command{
		noun: "app", verb: "package", order: 20,
		summary: "Store artifacts",
		targets: []string{"ios", "android", "macos", "windows"},
		usage: [][2]string{
			{"ios", "Signed .ipa"},
			{"android", "Signed .aab"},
			{"macos", "Signed .app, notarized; --dmg for a disk image"},
			{"windows", ".msix"},
			{"setup --check", "What each store needs, and what is missing"},
		},
		flags: [][2]string{
			{"--team <id>", "iOS: Apple Team ID"},
			{"--export-method <m>", "iOS: app-store, ad-hoc or development"},
			{"--keystore <path>", "Android: signing keystore"},
			{"--keystore-pass <s>", "Android: keystore password"},
			{"--key-alias <s>", "Android: key alias"},
			{"--key-pass <s>", "Android: key password"},
			{"--identity <cert>", "macOS: Developer ID Application certificate"},
			{"--notarize", "macOS: notarize the build"},
			{"--apple-id <email>", "macOS: Apple ID for notarization"},
			{"--password <s>", "macOS: app-specific password"},
			{"--dmg", "macOS: also produce a disk image"},
			{"--publisher <dn>", "Windows: publisher DN"},
			{"--cert <pfx>", "Windows: code-signing certificate"},
			{"--cert-pass <s>", "Windows: certificate password"},
			{"--version <v>", "Override common.version"},
			{"--icon <path>", "Override the source icon"},
			{"--output, -o", "Where to write the artifact"},
		},
		notes: `Every signing setting can be given on the command line as well as in
irgo.package.toml, which is how CI supplies secrets without writing them to
disk. Missing credentials are reported before the build rather than at the end
of it, and assets are regenerated first so a package cannot ship a stale
stylesheet.`,
	})
}
