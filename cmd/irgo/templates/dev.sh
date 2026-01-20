#!/bin/bash

# Irgo Development Script
# Watches templ files, builds tailwind CSS, and runs the Go server with hot reload

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}→${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}→${NC} $1"
}

# Check for required tools
check_requirements() {
    local missing=()

    if ! command -v go &> /dev/null; then
        missing+=("go")
    fi

    if ! command -v templ &> /dev/null; then
        missing+=("templ (go install github.com/a-h/templ/cmd/templ@latest)")
    fi

    if ! command -v air &> /dev/null; then
        missing+=("air (go install github.com/air-verse/air@latest)")
    fi

    if ! command -v entr &> /dev/null; then
        missing+=("entr (brew install entr)")
    fi

    if [ ${#missing[@]} -ne 0 ]; then
        echo -e "${RED}Missing required tools:${NC}"
        for tool in "${missing[@]}"; do
            echo "  - $tool"
        done
        exit 1
    fi
}

check_requirements

log_info "Irgo Development Server"
log_info "========================="

# Initial templ generation
log_info "Generating templ files..."
templ generate

# Check if we have node modules for tailwind
if [ -f "package.json" ]; then
    if [ ! -d "node_modules" ]; then
        log_info "Installing npm dependencies..."
        if command -v bun &> /dev/null; then
            bun install
        elif command -v npm &> /dev/null; then
            npm install
        else
            log_warn "No npm/bun found, skipping tailwind CSS build"
        fi
    fi

    # Initial CSS build
    log_info "Building Tailwind CSS..."
    if command -v bun &> /dev/null; then
        bun run css 2>/dev/null || true
    elif command -v npm &> /dev/null; then
        npm run css 2>/dev/null || true
    fi
fi

log_info "Starting development server on http://localhost:8080"
log_info "Press Ctrl+C to exit."
echo ""

# Run Tailwind CSS watcher in the background (if available)
if [ -f "package.json" ] && [ -d "node_modules" ]; then
    log_info "Starting Tailwind CSS watcher..."
    if command -v bun &> /dev/null; then
        bun run css:watch &
    elif command -v npm &> /dev/null; then
        npm run css:watch &
    fi
    TAILWIND_PID=$!
fi

# Watch for templ file changes and regenerate
log_info "Starting templ file watcher..."
find . -name "*.templ" -not -path "./node_modules/*" -not -path "./tmp/*" | entr -n bash -c 'echo "Templ file changed"; templ generate' &
ENTR_PID=$!

# Run air for Go hot reloading
log_info "Starting air (Go hot reload)..."
air

# Cleanup on exit
cleanup() {
    log_info "Cleaning up..."
    [ -n "$ENTR_PID" ] && kill $ENTR_PID 2>/dev/null
    [ -n "$TAILWIND_PID" ] && kill $TAILWIND_PID 2>/dev/null
}

trap cleanup EXIT
