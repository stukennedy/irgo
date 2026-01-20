//go:build desktop

package main

import (
	"flag"
	"fmt"
	"net/http"

	"github.com/stukennedy/gohtmx/desktop"
)

func main() {
	devMode := flag.Bool("dev", false, "Enable devtools")
	flag.Parse()

	r := setupRouter()
	addSampleData()

	// Create HTTP mux with static file serving
	mux := http.NewServeMux()
	staticDir := desktop.FindStaticDir()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	mux.Handle("/", r.Handler())

	// Configure desktop app
	config := desktop.DefaultConfig()
	config.Title = "Todo App"
	config.Debug = *devMode

	// Create and run desktop app
	app := desktop.New(mux, config)

	fmt.Println("Starting Todo desktop app...")
	if err := app.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
