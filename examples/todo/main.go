//go:build !desktop

// Example: Todo app demonstrating gohtmx framework usage with templ
// This file handles mobile and dev server modes.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/stukennedy/irgo/mobile"
)

func main() {
	// Check if running as desktop dev server or mobile initialization
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		runDevServer()
		return
	}

	// Mobile mode: initialize bridge
	initMobile()
}

// initMobile sets up the framework for mobile use
func initMobile() {
	mobile.Initialize()

	r := setupRouter()
	mobile.SetHandler(r.Handler())

	// Add sample data
	addSampleData()

	fmt.Println("Todo app initialized for mobile")
}

// runDevServer starts an HTTP server for desktop testing
func runDevServer() {
	r := setupRouter()

	// Add sample data
	addSampleData()

	port := ":8080"
	fmt.Printf("Starting dev server at http://localhost%s\n", port)
	log.Fatal(http.ListenAndServe(port, r.Handler()))
}
