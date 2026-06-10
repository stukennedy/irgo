package main

import "testing"

func TestSanitizeIdentifier(t *testing.T) {
	cases := map[string]string{
		"myapp":           "myapp",
		"my-app":          "myapp",
		"my_app":          "myapp",
		"My App":          "MyApp",
		"42birds":         "app42birds",
		"":                "app",
		"---":             "app",
		"irgo-xtool-test": "irgoxtooltest",
	}
	for in, want := range cases {
		if got := sanitizeIdentifier(in); got != want {
			t.Errorf("sanitizeIdentifier(%q) = %q, want %q", in, got, want)
		}
	}
}
