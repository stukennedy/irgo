package main

import (
	"strings"
	"testing"
)

func TestMergeGitignore(t *testing.T) {
	for _, tc := range []struct {
		name, have, want string
		added            int
		keeps            []string
		gains            []string
	}{{
		name:  "a new rule arrives with its comment",
		have:  "*_templ.go\n",
		want:  "*_templ.go\n\n# The SwiftPM shell for iOS on Linux.\nios/App/\n",
		added: 1,
		keeps: []string{"*_templ.go"},
		gains: []string{"ios/App/", "# The SwiftPM shell for iOS on Linux."},
	}, {
		// The failure this exists for: a project's own comment explaining a
		// decision, deleted by an upgrade that was adding something else.
		name: "the project's own notes survive",
		have: "*_templ.go\n\n# tokibundle/ is committed in full: bundle_gen.go holds\n" +
			"# the default locale and toki cannot start without it.\n\nnode_modules/\n",
		want:  "*_templ.go\nios/App/\n",
		added: 1,
		keeps: []string{
			"# tokibundle/ is committed in full: bundle_gen.go holds",
			"node_modules/",
		},
		gains: []string{"ios/App/"},
	}, {
		name:  "nothing new means no write",
		have:  "*_templ.go\nios/App/\n",
		want:  "# a comment\n*_templ.go\nios/App/\n",
		added: 0,
		keeps: []string{"*_templ.go"},
	}, {
		name:  "whitespace differences are not new rules",
		have:  "  *_templ.go  \n",
		want:  "*_templ.go\n",
		added: 0,
	}, {
		name:  "a negation is a rule like any other",
		have:  ".claude/*\n",
		want:  ".claude/*\n!.claude/skills/\n",
		added: 1,
		gains: []string{"!.claude/skills/"},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			got, n := mergeGitignore(tc.have, tc.want)
			if n != tc.added {
				t.Errorf("added %d rule(s), want %d\n\n%s", n, tc.added, got)
			}
			if n == 0 && got != tc.have {
				t.Errorf("nothing was added but the file changed:\n%s", got)
			}
			for _, k := range tc.keeps {
				if !strings.Contains(got, k) {
					t.Errorf("lost %q:\n%s", k, got)
				}
			}
			for _, g := range tc.gains {
				if !strings.Contains(got, g) {
					t.Errorf("did not gain %q:\n%s", g, got)
				}
			}
		})
	}
}

// TestMergeGitignoreIsIdempotent — upgrade runs repeatedly, and a merge that
// appended every time would grow the file without bound.
func TestMergeGitignoreIsIdempotent(t *testing.T) {
	have := "*_templ.go\n"
	want := "*_templ.go\n\n# native shell\nios/App/\n"

	once, n1 := mergeGitignore(have, want)
	twice, n2 := mergeGitignore(once, want)

	if n1 != 1 {
		t.Fatalf("first merge added %d, want 1", n1)
	}
	if n2 != 0 {
		t.Errorf("second merge added %d, want 0", n2)
	}
	if twice != once {
		t.Errorf("second merge changed the file:\n%s", twice)
	}
}
