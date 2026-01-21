.PHONY: all build ios android js clean test lint install-tools install help

# Go module
MODULE := github.com/stukennedy/irgo

# Output directories
BUILD_DIR := build
IOS_OUT := $(BUILD_DIR)/ios/Irgo.xcframework
ANDROID_OUT := $(BUILD_DIR)/android/irgo.aar

# Default target
all: build

# Build Go packages
build:
	go build ./...

# Run tests
test:
	go test -v ./...

# Run linter
lint:
	golangci-lint run ./...

# Install the irgo CLI
install:
	go install ./cmd/irgo
	@echo "irgo CLI installed. Run 'irgo new myapp' to create a new project."

# Install required tools
install-tools:
	go install github.com/a-h/templ/cmd/templ@latest
	go install golang.org/x/mobile/cmd/gomobile@latest
	go install github.com/air-verse/air@latest
	gomobile init
	@echo "Also install: brew install entr (for file watching)"

# Generate templ files
templ:
	templ generate

# Build iOS framework (requires Xcode)
ios: build
	@mkdir -p $(BUILD_DIR)/ios
	gomobile bind -target ios -o $(IOS_OUT) $(MODULE)/mobile
	@echo ""
	@echo "iOS framework built: $(IOS_OUT)"
	@echo ""
	@echo "To use in Xcode:"
	@echo "  1. Drag $(IOS_OUT) into your Xcode project"
	@echo "  2. Add to 'Frameworks, Libraries, and Embedded Content'"
	@echo "  3. Copy ios/Irgo/*.swift into your project"
	@echo "  4. Use IrgoWebViewController as your root view controller"

# Build Android AAR (requires Android SDK)
android: build
	@mkdir -p $(BUILD_DIR)/android
	gomobile bind -target android -o $(ANDROID_OUT) $(MODULE)/mobile
	@echo ""
	@echo "Android AAR built: $(ANDROID_OUT)"
	@echo ""
	@echo "To use in Android Studio:"
	@echo "  1. Copy $(ANDROID_OUT) to app/libs/"
	@echo "  2. Add to build.gradle: implementation files('libs/irgo.aar')"
	@echo "  3. Copy android/app/src/main/kotlin/com/irgo/*.kt to your project"
	@echo "  4. Extend IrgoActivity in your MainActivity"

# Bundle JavaScript
js:
	@mkdir -p $(BUILD_DIR)/js
	cp js/irgo-bridge.js $(BUILD_DIR)/js/
	@echo "JavaScript bundled: $(BUILD_DIR)/js/irgo-bridge.js"

# Build all platforms
mobile: ios android js
	@echo "All platforms built successfully"

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	go clean -cache

# Initialize a new irgo project
init-project:
	@echo "Creating project structure..."
	mkdir -p templates/{layouts,pages,fragments,components}
	mkdir -p assets/{css,js,images}
	mkdir -p handlers
	@echo "Copying Tailwind config..."
	@echo '$(shell cat pkg/render/tailwind.go | grep -A 100 "TailwindConfig =" | head -60)' > tailwind.config.js || true
	@echo "Project structure created. Run 'npm install' and 'npm run build:css' to set up Tailwind."

# Development server with hot reload (for desktop testing)
dev:
	./dev.sh

# Quick dev server (no watching, just run)
serve:
	go run ./examples/todo serve

# Format code
fmt:
	go fmt ./...
	gofmt -s -w .

# Tidy dependencies
tidy:
	go mod tidy

# Check for vulnerabilities
vuln:
	govulncheck ./...

# Generate documentation
docs:
	godoc -http=:6060 &
	@echo "Documentation server running at http://localhost:6060/pkg/$(MODULE)/"

# Version info
version:
	@echo "irgo framework"
	@go version
	@echo "Module: $(MODULE)"

# Release targets
release: release-patch

release-patch:
	@./scripts/release.sh patch

release-minor:
	@./scripts/release.sh minor

release-major:
	@./scripts/release.sh major

.PHONY: release release-patch release-minor release-major

# Help
help:
	@echo "Irgo Framework - Build Targets"
	@echo ""
	@echo "Development:"
	@echo "  make dev          - Run example app with hot reload"
	@echo "  make serve        - Quick run example (no watching)"
	@echo "  make test         - Run tests"
	@echo ""
	@echo "Mobile Builds:"
	@echo "  make ios          - Build iOS framework (.xcframework)"
	@echo "  make android      - Build Android library (.aar)"
	@echo "  make mobile       - Build all mobile platforms"
	@echo ""
	@echo "CLI:"
	@echo "  make install      - Install irgo CLI"
	@echo "  irgo new myapp    - Create new project"
	@echo ""
	@echo "Setup:"
	@echo "  make install-tools - Install dev tools (templ, gomobile, air)"
	@echo ""
	@echo "Release:"
	@echo "  make release       - Release patch version (0.2.2 -> 0.2.3)"
	@echo "  make release-minor - Release minor version (0.2.2 -> 0.3.0)"
	@echo "  make release-major - Release major version (0.2.2 -> 1.0.0)"
