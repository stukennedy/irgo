//go:build !desktop

// Example: Todo app demonstrating irgo framework usage with templ
// This file handles mobile and dev server modes.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/stukennedy/irgo/examples/todo/templates"
	"github.com/stukennedy/irgo/mobile"
	"github.com/stukennedy/irgo/pkg/livereload"
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

// runDevServer starts an HTTP server for development with live reload
func runDevServer() {
	// Enable dev mode for templates (enables live reload script)
	templates.DevMode = true

	r := setupRouter()
	lr := livereload.New()

	// Add sample data
	addSampleData()

	// Set up mux with live reload endpoint
	mux := http.NewServeMux()
	mux.HandleFunc("/dev/livereload", lr.Handler())
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("/", r.Handler())

	port := ":8080"
	fmt.Printf("Starting dev server at http://localhost%s\n", port)
	fmt.Printf("Live reload enabled (build time: %d)\n", lr.BuildTime())
	log.Fatal(http.ListenAndServe(port, mux))
}
