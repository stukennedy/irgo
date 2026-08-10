// The command registry: what the CLI can do, declared by the code that does it.
//
// This used to be one map holding every command, which meant every feature had
// to edit the same file. That is invisible until two people, or two branches,
// add a command at once — and then it is a conflict in a file neither of them
// is really working on, in a table that has nothing to do with either change.
//
// It showed up as an ordering problem rather than a merge problem: a set of
// changes with no logical dependency on each other could not be reviewed or
// merged independently, because they all touched this table. Seven branches,
// one real dependency between them, and a stack that had to land in sequence.
//
// So a command is declared next to its implementation and registers itself.
// Adding one touches the file that implements it and nothing else, and two
// features added in parallel do not meet.
//
// help.go, the usage index and the generated README all read what is
// registered, so a command that is not declared here has no help, no index
// entry and no README row — and a test fails rather than shipping any of it.
package main

import (
	"sort"
	"strings"
	"sync"
)

// command is one noun-verb pair.
type command struct {
	noun    string      // project, app, tools, ui, server
	verb    string      // new, build, doctor…
	order   int         // position within the noun, spaced by ten
	summary string      // one line: the index, the README, the title
	targets []string    // platform targets, shown as <a|b|c>
	args    string      // index hint when it takes no targets
	usage   [][2]string // {form, what it does}
	flags   [][2]string // {spec, what it does}
	notes   string      // the reasoning, rendered last
}

// commands is keyed "noun verb", filled by register at init.
var commands = map[string]command{}

// nouns is the order they appear in the usage index.
//
// A short list rather than a derived one: it is the shape of the CLI's
// grammar, not a detail of any command, and it changes when the grammar does
// — which is close to never.
var nouns = []string{"project", "app", "tools", "ui", "server"}

// register declares a command. Called from init in the file that implements
// it, so the declaration and the code cannot drift apart or be moved without
// each other.
//
// Duplicate keys panic rather than overwrite. Two files claiming one command
// is a mistake that would otherwise resolve by file order — which is to say,
// silently, and differently after a rename.
func register(c command) {
	key := c.noun + " " + c.verb
	if _, dup := commands[key]; dup {
		panic("irgo: command declared twice: " + key)
	}
	commands[key] = c
}

// nounVerbs lists what a noun accepts, in declaration order.
//
// Derived rather than listed. The list used to be written out by hand beside
// the dispatch switch, which made it a third place to update and the one most
// often forgotten: a verb missing from it dispatched correctly and appeared in
// no help at all.
//
// Ordered by the order field, then by verb, so the sequence does not depend on
// which file init happened to run first — that is filename order, which is
// arbitrary and changes when a file is renamed.
//
// Orders are spaced by ten. Consecutive numbers meant a command inserted in
// the middle had to renumber the ones after it, which is an edit to files the
// new feature has nothing to do with — the collision this whole change exists
// to remove, reappearing as an integer.
func nounVerbs(noun string) []string {
	var out []command
	for _, c := range allCommands() {
		if c.noun == noun {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].order != out[j].order {
			return out[i].order < out[j].order
		}
		return out[i].verb < out[j].verb
	})
	verbs := make([]string, len(out))
	for i, c := range out {
		verbs[i] = c.verb
	}
	return verbs
}

// buildTargets is what `app build` accepts, read from its declaration so the
// dispatch, the help and the README cannot disagree about it.
//
// A function, not a package-level variable. Go initialises variables before it
// runs init, so a var reading this map would have read an empty one — and the
// symptom is a CLI that advertises no build targets at all.
func buildTargets() []string {
	c, _ := lookup("app build")
	return c.targets
}

// targetList renders a verb's targets for a usage line.
func targetList(nounVerb string) string {
	c, _ := lookup(nounVerb)
	return strings.Join(c.targets, "|")
}

func buildTargetList() string { return targetList("app build") }

// registerTarget adds a platform to an existing command, from the file that
// implements that platform.
//
// A target is not a command, and this is the case register alone did not
// cover: `app build web` extends `app build` rather than declaring anything
// new. Without this, adding a target meant editing the declaration of a
// command owned by another feature — the same collision as the old central
// table, only smaller and less obvious.
//
// Appended in registration order after the ones the command declares itself,
// so the common targets keep the order they were written in and additions
// arrive after them.
func registerTarget(nounVerb, target, usage string, run func(args []string) error) {
	// Queued, not applied. init runs in filename order, so app_web_build.go
	// registers its target before cmd_app_build.go has declared the command it
	// extends — and applying immediately panicked on a command that was about
	// to exist. Nothing may depend on init order; that is filename order, and
	// it changes when a file is renamed.
	pendingTargets = append(pendingTargets, pendingTarget{nounVerb, target, usage})
	if run != nil {
		targetRuns[nounVerb+" "+target] = run
	}
}

type pendingTarget struct{ nounVerb, target, usage string }

var pendingTargets []pendingTarget

// resolved applies queued targets once every init has run.
var resolved sync.Once

// resolveTargets folds the queued targets into their commands.
func resolveTargets() {
	resolved.Do(func() {
		for _, p := range pendingTargets {
			c, ok := commands[p.nounVerb]
			if !ok {
				panic("irgo: registerTarget for unknown command: " + p.nounVerb)
			}
			for _, t := range c.targets {
				if t == p.target {
					panic("irgo: target declared twice: " + p.nounVerb + " " + p.target)
				}
			}
			c.targets = append(c.targets, p.target)
			if p.usage != "" {
				c.usage = append(c.usage, [2]string{p.target, p.usage})
			}
			commands[p.nounVerb] = c
		}
	})
}

// lookup is the only way to read a command, so queued targets are always
// applied first. Reading the map directly would see a command without the
// targets another file added to it.
func lookup(key string) (command, bool) {
	resolveTargets()
	c, ok := commands[key]
	return c, ok
}

// allCommands is every registered command, targets resolved.
func allCommands() map[string]command {
	resolveTargets()
	return commands
}

// targetRuns holds what a registered target does, so dispatch does not need a
// case in a switch owned by someone else.
//
// This is the other half of registerTarget. Declaring a target without being
// able to dispatch it still meant editing a shared file — a smaller collision
// than the command table, and the same kind.
var targetRuns = map[string]func(args []string) error{}

// runTarget dispatches a registered target, reporting whether there was one.
func runTarget(nounVerb, target string, args []string) (error, bool) {
	resolveTargets()
	if run, ok := targetRuns[nounVerb+" "+target]; ok {
		return run(args), true
	}
	return nil, false
}
