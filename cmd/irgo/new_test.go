package main

import "testing"

func TestSanitizeIdentifier(t *testing.T) {
	cases := map[string]string{
		"myapp":           "myapp",
		"my-app":          "my_app",
		"my_app":          "my_app",
		"My App":          "My_App",
		"123game":         "app123game",
		"":                "app",
		"---":             "___",
		"irgo-xtool-test": "irgo_xtool_test",
	}
	for in, want := range cases {
		if got := sanitizeIdentifier(in); got != want {
			t.Errorf("sanitizeIdentifier(%q) = %q, want %q", in, got, want)
		}
	}
}
