.PHONY: dev serve build test clean templ ios android help

# Development server with hot reload
dev:
	@if command -v irgo >/dev/null 2>&1; then \
		irgo dev; \
	else \
		./dev.sh; \
	fi

# Quick server (no watching)
serve:
	go run . serve

# Build binary
build:
	templ generate
	go build -o bin/app .

# Run tests
test:
	go test -v ./...

# Generate templ files
templ:
	templ generate

# Build for iOS (requires gomobile)
ios:
	@if command -v irgo >/dev/null 2>&1; then \
		irgo build ios; \
	else \
		templ generate && \
		mkdir -p build/ios && \
		gomobile bind -target ios -o build/ios/App.xcframework ./mobile; \
	fi

# Build for Android (requires gomobile)
android:
	@if command -v irgo >/dev/null 2>&1; then \
		irgo build android; \
	else \
		templ generate && \
		mkdir -p build/android && \
		gomobile bind -target android -o build/android/app.aar ./mobile; \
	fi

# Clean build artifacts
clean:
	rm -rf bin/ build/ tmp/
	go clean

# Install development tools
install-tools:
	go install github.com/a-h/templ/cmd/templ@latest
	go install github.com/air-verse/air@latest
	go install golang.org/x/mobile/cmd/gomobile@latest
	go install github.com/stukennedy/irgo/cmd/irgo@latest
	@echo "Also install: brew install entr"

# Show help
help:
	@echo "Available targets:"
	@echo "  dev     - Run dev server with hot reload"
	@echo "  serve   - Run server (no watching)"
	@echo "  build   - Build Go binary"
	@echo "  test    - Run tests"
	@echo "  templ   - Generate templ files"
	@echo "  ios     - Build iOS framework"
	@echo "  android - Build Android AAR"
	@echo "  clean   - Remove build artifacts"
