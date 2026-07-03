//go:build !desktop

package desktop

import "fmt"

// webviewHandle is a placeholder in non-desktop builds so the App struct
// compiles (and its non-webview functionality remains testable) without the
// CGO webview dependency.
type webviewHandle interface{}

var errNotDesktopBuild = fmt.Errorf(
	"desktop: built without the 'desktop' build tag; build with -tags desktop (irgo run/build desktop does this automatically)")

// runWebview reports that this binary was built without desktop support.
func (a *App) runWebview() error {
	return errNotDesktopBuild
}

// Bind is unavailable without the desktop build tag.
func (a *App) Bind(name string, fn interface{}) error {
	return errNotDesktopBuild
}

// Eval is a no-op without the desktop build tag.
func (a *App) Eval(js string) {}
