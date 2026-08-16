// Help text for every command.
//
// Separated from dispatch because main.go was 916 lines, of which two thirds
// were strings: the switch that routes a command and the prose describing it
// change for different reasons and at different times.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func printUsage() {
	fmt.Println(`irgo - one Go codebase for web, desktop, iOS and Android

Usage:
  irgo <noun> <verb> [target] [flags]

One grammar throughout: every command is a noun and a verb.

` + renderNounSections() + `  version                  Print version information
  help [command]           Detail for one command

Examples:
  irgo project new myapp             Create a project
  irgo server dev                    Hot-reload server on :8080
  irgo tools doctor                  What can this machine build?
  irgo app run ios                   iOS Simulator
  irgo app run ios --device          A USB-connected iPhone
  irgo app build desktop all         Every desktop target this host supports
  irgo app package macos --dmg       Signed .app and a DMG
  irgo app install desktop           Put the built app in /Applications
  irgo app deploy cloudflare         Live on Cloudflare Workers, SSE included
  irgo tools remove --yes            Undo everything irgo installed
  irgo project config ios.team       Which team signs, and what is available
  irgo project pin local ../irgo     Build a checkout you are editing

Nothing needs installing first: toolchains provision themselves when a command
needs them, and go.mod pins the CLI so ` + "`go tool irgo`" + ` always matches the project.`)
}

// printCommandHelp explains one command, rendered from its declaration in
// commands.go. Nothing is typed twice: usage lines, flags and column widths
// all come from the same data the index and the README read.
func printCommandHelp(noun, verb string) {
	key := strings.TrimSpace(noun + " " + verb)

	if c, ok := lookup(key); ok {
		printCommand(key, c)
		return
	}

	// A noun on its own lists its verbs. Checked against the registry rather
	// than a list written out here: a noun a feature adds — `ui`, when a
	// component kit is wired in — would otherwise fall through to "no help",
	// while its verbs worked perfectly.
	if verb == "" && len(nounVerbs(noun)) > 0 {
		fmt.Printf("irgo %s - %s\n\n", noun, nounSummary[noun])
		fmt.Println("Verbs:")
		fmt.Println(renderNounVerbs(noun, "irgo "))
		fmt.Printf("\nDetail for one:  irgo help %s <verb>\n", noun)
		return
	}

	switch key {
	case "version":
		fmt.Println(`irgo version - Print the version

Usage:
  irgo version

Prints the CLI version and, in a project, which irgo go.mod resolves to — a
release, a fork tag or a local checkout.`)

	default:
		fmt.Printf("No help for %q.\n\n", key)
		printUsage()
	}
}

// printCommand renders one command's page.
func printCommand(key string, c command) {
	fmt.Printf("irgo %s - %s\n", key, c.summary)

	if len(c.usage) > 0 {
		fmt.Println()
		fmt.Println("Usage:")
		width := 0
		for _, u := range c.usage {
			if n := len("irgo " + key + " " + u[0]); n > width {
				width = n
			}
		}
		for _, u := range c.usage {
			left := strings.TrimRight("irgo "+key+" "+u[0], " ")
			fmt.Printf("  %-*s  %s\n", width, left, u[1])
		}
	}

	if len(c.flags) > 0 {
		fmt.Println()
		fmt.Println("Flags:")
		width := 0
		for _, f := range c.flags {
			if len(f[0]) > width {
				width = len(f[0])
			}
		}
		for _, f := range c.flags {
			fmt.Printf("  %-*s  %s\n", width, f[0], f[1])
		}
	}

	if c.notes != "" {
		fmt.Println()
		fmt.Println(c.notes)
	}
}

// renderNounSections writes the usage index, one block per noun.
//
// Derived rather than five hardcoded blocks. A noun added by a feature — `ui`,
// when a UI kit is wired in — appeared in its own help and in nothing else,
// because the front page listed the nouns it happened to be written with. That
// is the same coupling the command table had, one level up.
//
// A noun with nothing registered is skipped, so a noun declared ahead of the
// feature that fills it does not print an empty heading.
func renderNounSections() string {
	var b strings.Builder
	for _, noun := range nouns {
		verbs := nounVerbs(noun)
		if len(verbs) == 0 {
			continue
		}
		b.WriteString(strings.ToUpper(noun))
		for i := len(noun); i < 10; i++ {
			b.WriteString(" ")
		}
		b.WriteString(nounSummary[noun] + "\n")
		b.WriteString(renderNounSection(noun))
		b.WriteString("\n\n")
	}
	return b.String()
}

// nounSummary is the one-line description of each noun, used when help is
// asked for a noun rather than a command.
var nounSummary = map[string]string{
	"project": "the repository you are in",
	"app":     "what gets built, run, shipped and installed",
	"tools":   "the toolchains on this machine",
	"server":  "the development server",
	"ui":      "the component kit, when a project uses one",
}

// renderCommandTable writes the command reference that ships in the generated
// README, from the same declarations the CLI dispatches on.
func renderCommandTable() string {
	var b strings.Builder
	b.WriteString("| Command | What it does |\n|---|---|\n")
	for _, noun := range nouns {
		for _, verb := range nounVerbs(noun) {
			key := noun + " " + verb
			c, _ := lookup(key)
			name := key
			if len(c.targets) > 0 {
				name += " <" + strings.Join(c.targets, "|") + ">"
			}
			fmt.Fprintf(&b, "| `%s` | %s |\n", name, c.summary)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderNounSection writes a noun's verbs for the usage index, with the
// targets or arguments each accepts and the columns lined up.
func renderNounSection(noun string) string { return renderNounVerbs(noun, "") }

// renderNounVerbs is the same listing with an optional prefix: the index
// writes bare command names, a noun's own help page writes runnable ones.
func renderNounVerbs(noun, prefix string) string {
	type row struct{ left, right string }
	var rows []row
	width := 0
	for _, verb := range nounVerbs(noun) {
		c, _ := lookup(noun + " " + verb)
		left := prefix + noun + " " + verb
		if len(c.targets) > 0 {
			left += " <" + strings.Join(c.targets, "|") + ">"
		} else if c.args != "" {
			left += " " + c.args
		}
		if len(left) > width {
			width = len(left)
		}
		rows = append(rows, row{left, c.summary})
	}
	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, r.left, r.right)
	}
	return strings.TrimRight(b.String(), "\n")
}

// printCommandsJSON writes the command declarations as JSON.
//
// The documentation site's CLI reference was written by hand and went stale
// every time a command moved — it described `irgo doctor` and `irgo ios team`
// long after both were gone. Reading the CLI's own declarations means the page
// cannot say something the binary does not do.
func printCommandsJSON() {
	type flagDoc struct {
		Spec string `json:"spec"`
		What string `json:"what"`
	}
	type usageDoc struct {
		Form string `json:"form"`
		What string `json:"what"`
	}
	type commandDoc struct {
		Noun    string     `json:"noun"`
		Verb    string     `json:"verb"`
		Summary string     `json:"summary"`
		Targets []string   `json:"targets,omitempty"`
		Args    string     `json:"args,omitempty"`
		Usage   []usageDoc `json:"usage,omitempty"`
		Flags   []flagDoc  `json:"flags,omitempty"`
		Notes   string     `json:"notes,omitempty"`
	}

	var out []commandDoc
	for _, noun := range nouns {
		for _, verb := range nounVerbs(noun) {
			c, _ := lookup(noun + " " + verb)
			d := commandDoc{
				Noun: noun, Verb: verb, Summary: c.summary,
				Targets: c.targets, Args: c.args, Notes: c.notes,
			}
			for _, u := range c.usage {
				d.Usage = append(d.Usage, usageDoc{Form: u[0], What: u[1]})
			}
			for _, f := range c.flags {
				d.Flags = append(d.Flags, flagDoc{Spec: f[0], What: f[1]})
			}
			out = append(out, d)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
