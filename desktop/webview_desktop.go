//go:build desktop

package desktop

import (
	"fmt"

	webview "github.com/webview/webview_go"
)

// webviewHandle is the real webview in desktop builds.
type webviewHandle = webview.WebView

// runWebview opens the native window and blocks until it is closed.
func (a *App) runWebview() error {
	a.wv = webview.New(a.config.Debug)
	defer a.wv.Destroy()

	a.wv.SetTitle(a.config.Title)

	if a.config.Resizable {
		a.wv.SetSize(a.config.Width, a.config.Height, webview.HintNone)
	} else {
		a.wv.SetSize(a.config.Width, a.config.Height, webview.HintFixed)
	}

	// Inject the secret into the webview before navigation
	// Using Init() ensures the script runs before any page scripts
	cfg := a.transport.Config()
	if cfg != nil && cfg.Secret != "" {
		js := "window.__IRGO_SECRET__ = '" + cfg.Secret + "';"
		a.wv.Init(js)
	}

	// Navigate to the server URL
	url := a.URL()
	if url != "" {
		a.wv.Navigate(url)
	}

	// Run blocks until window is closed
	a.wv.Run()
	return nil
}

// Bind binds a Go function to a JavaScript name in the webview
func (a *App) Bind(name string, fn interface{}) error {
	if a.wv == nil {
		return fmt.Errorf("webview not initialized")
	}
	a.wv.Bind(name, fn)
	return nil
}

// Eval evaluates JavaScript in the webview
func (a *App) Eval(js string) {
	if a.wv != nil {
		a.wv.Eval(js)
	}
}
