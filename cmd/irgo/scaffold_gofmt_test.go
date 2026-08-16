// What irgo generates must be formatted, for any project name.
//
// A developer running `gofmt -l .` on a project made five seconds ago should
// see nothing. Before this, they saw the files irgo had just written.
//
// The name matters, which is what makes this worth a test rather than a glance
// at the templates. A scaffolded file imports both the framework and the new
// project, and gofmt sorts those alphabetically, so `github.com/stukennedy/…`
// comes before `zebra/app` and after `alpha/app`. No fixed order in the
// template is right for both. So both are checked here.
package main

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldedGoIsFormatted(t *testing.T) {
	// One name that sorts before the framework's import path and one that
	// sorts after. A template hand-sorted for either fails on the other.
	for _, project := range []string{"alpha", "zebra"} {
		t.Run(project, func(t *testing.T) {
			err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go.tmpl") {
					return err
				}
				body, err := templateFS.ReadFile(path)
				if err != nil {
					return err
				}

				got := renderTemplate(string(body), project, project, "")
				want := gofmtIfGo(got)
				if got != want {
					t.Errorf("%s is not gofmt'd once rendered for module %q\n\n%s",
						filepath.Base(path), project, firstDifference(got, want))
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walking templates: %v", err)
			}
		})
	}
}

// firstDifference reports the first line that differs, because a whole-file
// dump of a scaffolded main.go buries the one import that moved.
func firstDifference(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(g) && i < len(w); i++ {
		if g[i] != w[i] {
			return "line " + itoa(i+1) + ":\n  have: " + g[i] + "\n  want: " + w[i]
		}
	}
	return "the files differ in length"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
