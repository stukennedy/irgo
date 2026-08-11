// Adding the framework's ignore rules without deleting yours.
//
// .gitignore is the one framework-owned file a project also writes in. irgo
// decides which paths are generated, so an upgrade that adds one has to add
// its rule — and a developer adds their own build output, their editor's
// directory, and the occasional comment explaining a decision that took an
// afternoon to reach.
//
// Replacing the file served the first of those and silently destroyed the
// second.
package main

import "strings"

// mergeGitignore returns the project's file with any rules the template has
// and it does not, and the number added.
//
// Only ever appends. A rule the project deleted on purpose comes back, which
// is the lesser wrong: irgo cannot tell a deliberate deletion from a file that
// predates the rule, and the failure modes are not symmetric — a returning
// rule is visible in the diff, while a deleted one shows up as a generated
// file committed three weeks later.
func mergeGitignore(have, want string) (string, int) {
	existing := map[string]bool{}
	for _, line := range strings.Split(have, "\n") {
		if p := strings.TrimSpace(line); p != "" && !strings.HasPrefix(p, "#") {
			existing[p] = true
		}
	}

	// Comments travel with the rule below them. A bare pattern appended
	// without its explanation is how .gitignore files become lists of paths
	// nobody dares remove.
	var pending, added []string
	count := 0
	for _, line := range strings.Split(want, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			pending = nil // a blank line ends a comment block
		case strings.HasPrefix(trimmed, "#"):
			pending = append(pending, line)
		case existing[trimmed]:
			pending = nil // already covered; its comment is not news either
		default:
			added = append(added, pending...)
			added = append(added, line)
			pending = nil
			count++
		}
	}
	if count == 0 {
		return have, 0
	}

	out := strings.TrimRight(have, "\n")
	out += "\n\n# Added by irgo project upgrade.\n"
	out += strings.Join(added, "\n") + "\n"
	return out, count
}
