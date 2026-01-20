#!/bin/bash

# Irgo Framework Development Script
# This runs the example todo app for framework development/testing
# For your own projects, use: irgo new myapp

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

# Check if we have node modules for tailwind
if [ -f "package.json" ] && [ ! -d "node_modules" ]; then
    log_info "Installing npm dependencies..."
    if command -v bun &> /dev/null; then
        bun install
    elif command -v npm &> /dev/null; then
        npm install
    else
        log_warn "No npm/bun found, tailwindcss may not work"
    fi
fi

log_info "Starting development server on http://localhost:8080"
log_info "Air will handle templ generation and Tailwind CSS builds"
log_info "Press Ctrl+C to exit."
echo ""

# Run air for Go hot reloading (also runs templ generate and tailwindcss)
air
