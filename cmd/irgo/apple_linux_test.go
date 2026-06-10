package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClangTargetForSDK(t *testing.T) {
	cases := map[string]string{
		"iphoneos":        "arm64-apple-ios",
		"iphonesimulator": "arm64-apple-ios-simulator",
		"macosx":          "arm64-apple-macos",
		"watchos":         "",
	}
	for sdk, want := range cases {
		if got := clangTargetForSDK(sdk); got != want {
			t.Errorf("clangTargetForSDK(%q) = %q, want %q", sdk, got, want)
		}
	}
}

func TestAppleToolchainSDKPath(t *testing.T) {
	tc := &appleToolchain{artifactBundle: "/sdk/darwin.artifactbundle"}
	cases := map[string]string{
		"iphoneos":        "/sdk/darwin.artifactbundle/Developer/Platforms/iPhoneOS.platform/Developer/SDKs/iPhoneOS.sdk",
		"iphonesimulator": "/sdk/darwin.artifactbundle/Developer/Platforms/iPhoneSimulator.platform/Developer/SDKs/iPhoneSimulator.sdk",
		"macosx":          "/sdk/darwin.artifactbundle/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk",
		"watchos":         "",
	}
	for sdk, want := range cases {
		if got := tc.sdkPath(sdk); got != filepath.FromSlash(want) && got != want {
			t.Errorf("sdkPath(%q) = %q, want %q", sdk, got, want)
		}
	}
}

func TestWriteAppleShims(t *testing.T) {
	dir := t.TempDir()
	tc := &appleToolchain{
		artifactBundle: "/sdk/darwin.artifactbundle",
		libtool:        "/sdk/darwin.artifactbundle/toolset/bin/libtool",
		clang:          "/usr/bin/clang",
	}
	if err := writeAppleShims(dir, tc); err != nil {
		t.Fatalf("writeAppleShims: %v", err)
	}

	for _, name := range []string{
		"xcrun", "xcodebuild", "lipo",
		"clang-iphoneos", "clang-iphonesimulator", "clang-macosx",
	} {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected shim %s: %v", name, err)
		}
		if info.Mode().Perm()&0o100 == 0 {
			t.Errorf("shim %s is not executable", name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(data), "#!/bin/sh") {
			t.Errorf("shim %s missing shebang", name)
		}
	}

	// The iphoneos clang wrapper must target arm64-apple-ios with the SDK
	// from the artifact bundle.
	data, _ := os.ReadFile(filepath.Join(dir, "clang-iphoneos"))
	if !strings.Contains(string(data), "-target arm64-apple-ios") {
		t.Error("clang-iphoneos wrapper missing -target arm64-apple-ios")
	}
	if !strings.Contains(string(data), "iPhoneOS.sdk") {
		t.Error("clang-iphoneos wrapper missing iPhoneOS.sdk sysroot")
	}

	// The xcrun shim must answer --show-sdk-path for every SDK gomobile
	// probes at startup.
	data, _ = os.ReadFile(filepath.Join(dir, "xcrun"))
	for _, sdk := range []string{"iphoneos", "iphonesimulator", "macosx"} {
		if !strings.Contains(string(data), sdk+")") {
			t.Errorf("xcrun shim missing case for sdk %s", sdk)
		}
	}
}

func TestSanitizeBundleIdent(t *testing.T) {
	cases := map[string]string{
		"myapp":           "myapp",
		"my-app":          "myapp",
		"my_app":          "myapp",
		"My App":          "MyApp",
		"123game":         "app123game",
		"":                "app",
		"---":             "app",
		"irgo-xtool-test": "irgoxtooltest",
	}
	for in, want := range cases {
		if got := sanitizeBundleIdent(in); got != want {
			t.Errorf("sanitizeBundleIdent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadXtoolBundleID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xtool.yml")
	content := "# comment\nversion: 1\nbundleID: com.irgo.myapp\ninfoPath: Info.plist\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readXtoolBundleID(path)
	if err != nil {
		t.Fatalf("readXtoolBundleID: %v", err)
	}
	if got != "com.irgo.myapp" {
		t.Errorf("bundleID = %q, want com.irgo.myapp", got)
	}

	// Missing bundleID should error.
	noID := filepath.Join(dir, "empty.yml")
	if err := os.WriteFile(noID, []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readXtoolBundleID(noID); err == nil {
		t.Error("expected error for xtool.yml without bundleID")
	}
}
