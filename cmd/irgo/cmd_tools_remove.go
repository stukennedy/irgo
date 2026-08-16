// Removing what irgo installed.
//
// This deletes gigabytes — the Android SDK and NDK, a JDK, Homebrew formulae —
// so it plans first, shows exactly what it will touch, and asks. A command that
// destroys that much on a bare invocation is a trap, however accurate its
// output afterwards.
//
// Android is not a separate command. "Remove what irgo installed" that leaves
// behind the largest thing irgo installed is a promise the CLI does not keep,
// and it is the part someone will forget.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// removalPlan is what a removal would do, computed before anything is deleted.
type removalPlan struct {
	// group -> lines to show under it.
	groups []planGroup
	// kept lists things irgo did not install, which are reported and skipped.
	kept []string
	acts []func(*removalTally)
}

type planGroup struct {
	name  string
	lines []string
}

func (p *removalPlan) add(group, line string, act func(*removalTally)) {
	for i := range p.groups {
		if p.groups[i].name == group {
			p.groups[i].lines = append(p.groups[i].lines, line)
			p.acts = append(p.acts, act)
			return
		}
	}
	p.groups = append(p.groups, planGroup{name: group, lines: []string{line}})
	p.acts = append(p.acts, act)
}

func (p *removalPlan) empty() bool { return len(p.acts) == 0 }

// confirmRemoval shows the plan and asks. Outside a terminal it refuses rather
// than assuming yes: a script that deletes an SDK because nobody was watching
// is worse than one that stops and says what it needs.
func confirmRemoval(p *removalPlan, yes bool) (bool, error) {
	fmt.Println("This will remove what irgo installed on this machine:")
	for _, g := range p.groups {
		fmt.Println()
		fmt.Printf("%s:\n", g.name)
		for _, l := range g.lines {
			fmt.Printf("  %s\n", l)
		}
	}
	if len(p.kept) > 0 {
		fmt.Println()
		fmt.Printf("Kept (irgo did not install these): %s\n", strings.Join(p.kept, ", "))
		fmt.Println("  --all removes them too.")
	}
	fmt.Println()

	if yes {
		return true, nil
	}
	if !interactive() {
		return false, fmt.Errorf("refusing to remove without confirmation.\n" +
			"  Nothing has been deleted. Re-run with --yes to proceed:\n" +
			"    irgo tools remove --yes")
	}
	fmt.Print("Proceed? [y/N] ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	}
	fmt.Println("Nothing removed.")
	return false, nil
}

// planGoTools adds the go-installed tools irgo owns.
func planGoTools(p *removalPlan, all bool) {
	for _, tool := range []string{"templ", "air", "gomobile", "gobind"} {
		t := tool
		if !all && !toolInstalledByIrgo(t) {
			if pathOnPATH(t) {
				p.kept = append(p.kept, t)
			}
			continue
		}
		path := ""
		for _, dir := range toolBinDirs() {
			candidate := filepath.Join(dir, exeName(t))
			if pathExists(candidate) {
				path = candidate
				break
			}
		}
		if path == "" {
			continue
		}
		p.add("Go tools", fmt.Sprintf("%-14s %s", t, path), func(tally *removalTally) {
			if q := removeToolPath(t); q != "" {
				tally.report(t, "removed", q)
			} else {
				tally.report(t, "absent", "")
			}
			clearToolMarker(t)
		})
	}
}

func pathOnPATH(name string) bool {
	for _, dir := range toolBinDirs() {
		if pathExists(filepath.Join(dir, exeName(name))) {
			return true
		}
	}
	return false
}

// planDownloads adds the pinned binaries under ~/.irgo.
func planDownloads(p *removalPlan) {
	bin := filepath.Join(homeDir(), ".irgo", "bin")
	entries, err := os.ReadDir(bin)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "tailwindcss") {
			continue
		}
		p.add("Downloads", fmt.Sprintf("%-14s %s", "tailwindcss", filepath.Join(bin, e.Name())),
			func(tally *removalTally) {
				if q := removeTailwindPath(); q != "" {
					tally.report("tailwindcss", "removed", q)
				} else {
					tally.report("tailwindcss", "absent", "")
				}
			})
		return
	}
}

// planNode adds the Node irgo downloads for wrangler. It is large and it is
// only there for Cloudflare deploys, so leaving it behind after a removal
// would be the kind of quiet leftover this command exists to prevent.
func planNode(p *removalPlan) {
	dir := managedNodeHome()
	if !isDir(dir) {
		return
	}
	p.add("Downloads", fmt.Sprintf("%-14s %s%s", "node", dir, dirSizeNote(dir)),
		func(tally *removalTally) {
			if err := os.RemoveAll(dir); err != nil {
				tally.report("node", "failed", err.Error())
				return
			}
			clearToolMarker("node")
			tally.report("node", "removed", dir)
		})
}

// planHostPackages adds packages installed through the host package manager.
func planHostPackages(p *removalPlan, all bool) {
	mgr := pkgManager()
	if mgr == "" {
		return
	}
	for _, pkg := range osPackages() {
		pk := pkg
		name := pk.pkgNameFor(mgr)
		if name == "" || !pk.probe() {
			continue
		}
		if !all && !toolInstalledByIrgo(pk.key) {
			p.kept = append(p.kept, pk.key)
			continue
		}
		p.add(fmt.Sprintf("Host packages (%s)", mgr), fmt.Sprintf("%-14s %s", pk.key, name),
			func(tally *removalTally) {
				cmd := pkgRemoveCmd(mgr, name)
				if out, err := runCommandQuiet(cmd[0], cmd[1:]...); err != nil {
					tally.report(pk.key, "failed", firstLine(strings.TrimSpace(out)))
				} else {
					tally.report(pk.key, "removed", "via "+mgr)
				}
				clearToolMarker(pk.key)
			})
	}
}

// planAndroid adds the Android toolchain, which is by far the largest thing
// irgo installs and therefore the one most worth showing before deleting.
func planAndroid(p *removalPlan, keepJDK bool) {
	home := homeDir()
	sdk := androidHome()

	// Only an SDK irgo provisioned carries the marker; never delete a
	// developer's own.
	if pathExists(filepath.Join(sdk, toolchainMarker)) {
		p.add("Android", fmt.Sprintf("%-14s %s%s", "SDK/NDK", sdk, dirSizeNote(sdk)), func(tally *removalTally) {
			if os.RemoveAll(sdk) == nil {
				tally.report("android-sdk", "removed", sdk)
			} else {
				tally.report("android-sdk", "failed", sdk)
			}
		})
	} else if pathExists(sdk) {
		p.kept = append(p.kept, "android SDK (not provisioned by irgo)")
	}

	jdk := filepath.Join(home, ".irgo", "jdks")
	if !keepJDK && pathExists(jdk) {
		p.add("Android", fmt.Sprintf("%-14s %s%s", "JDK 17", jdk, dirSizeNote(jdk)), func(tally *removalTally) {
			if os.RemoveAll(jdk) == nil {
				tally.report("managed-jdk", "removed", jdk)
			} else {
				tally.report("managed-jdk", "failed", jdk)
			}
		})
	}

	for _, c := range []struct{ label, path string }{
		{"AVDs/adb", filepath.Join(home, ".android")},
		{"Gradle cache", filepath.Join(home, ".gradle")},
	} {
		cc := c
		if !pathExists(cc.path) {
			continue
		}
		p.add("Android", fmt.Sprintf("%-14s %s%s", cc.label, cc.path, dirSizeNote(cc.path)), func(tally *removalTally) {
			if os.RemoveAll(cc.path) == nil {
				tally.report(cc.label, "removed", cc.path)
			} else {
				tally.report(cc.label, "failed", cc.path)
			}
		})
	}
}

// dirSizeNote reports an approximate size, because "this deletes 4 GB" is the
// fact that changes whether someone says yes.
func dirSizeNote(path string) string {
	out, err := runCommandQuiet("du", "-sh", path)
	if err != nil {
		return ""
	}
	if f := strings.Fields(out); len(f) > 0 {
		return "  (" + f[0] + ")"
	}
	return ""
}

func init() {
	register(command{
		noun: "tools", verb: "remove", order: 10,
		summary: "Undo it — shows what it will delete, and asks",
		args:    "[android] [--all] [--yes]",
		usage: [][2]string{
			{"", "What irgo installed for this host"},
			{"android", "The Android toolchain"},
			{"--all", "Everything, including the SDK and AVDs"},
		},
		flags: [][2]string{
			{"--all", "Everything irgo installed, including the SDK and AVDs"},
			{"--keep-jdk", "Leave the managed JDK in place"},
			{"--yes, -y", "Do not ask"},
		},
		notes: `Shows what it will delete, with sizes, and asks first. Outside a terminal it
refuses rather than assuming yes.

Only what irgo installed is removed: each install leaves a marker, and anything
without one is left alone, so a toolchain you set up yourself survives.`,
	})
}
