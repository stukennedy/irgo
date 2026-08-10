// `irgo app deploy cloudflare` — build the Worker and put it live.
//
// The two steps were already possible by hand, and by hand is where they went
// wrong: the build must regenerate the shim in go mode (the generator defaults
// to tinygo, and mixing them produces a Worker that throws 1101 on every
// route), and wrangler must run on Node (Cloudflare does not support bun, and a
// machine whose `node` is a version-manager shim with no default has one that
// fails rather than one that works). Both are things to know rather than
// things to remember.
package main

import (
	"fmt"
	"os"
	"strings"
)

func deployCloudflare(modulePath string) error {
	deploying = true
	// runBuild, not buildCloudflare: deploying has to be building plus
	// uploading, and nothing else. Calling the inner function skipped the
	// asset regeneration that every other build path does, so a deploy could
	// ship a stale stylesheet — the exact failure `app package` documents as
	// impossible, reintroduced by taking a shortcut one layer down.
	if err := runBuild("cloudflare", false, false, ""); err != nil {
		return err
	}

	if err := checkCloudflareCredentials(); err != nil {
		return err
	}

	node, err := nodeBin(true)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Deploying to Cloudflare...")
	cmd := npxCommand(node, "--yes", "wrangler@4", "deploy")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wrangler deploy failed: %w", err)
	}
	return nil
}

// checkCloudflareCredentials fails before a build's worth of work is uploaded
// to nothing. wrangler will also prompt for an interactive login, which is
// fine on a laptop and hangs forever in CI, so the absence is named here.
func checkCloudflareCredentials() error {
	if os.Getenv("CLOUDFLARE_API_TOKEN") != "" {
		return nil
	}
	// An interactive terminal can log in through the browser instead.
	if interactive() {
		return nil
	}
	return fmt.Errorf("CLOUDFLARE_API_TOKEN is not set, and there is no terminal to log in from.\n" +
		"  In CI, set it as a secret.\n" +
		"  Locally, either export it or run this from a terminal and wrangler will open a browser.")
}

// cloudflareWorkerName reads the name out of wrangler.toml so messages can
// say which Worker is meant.
func cloudflareWorkerName() string {
	data, err := os.ReadFile("wrangler.toml")
	if err != nil {
		return ""
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if after, ok := strings.CutPrefix(line, "name"); ok {
			if _, v, found := strings.Cut(after, "="); found {
				return strings.Trim(strings.TrimSpace(v), `"'`)
			}
		}
	}
	return ""
}

func init() {
	register(command{
		noun: "app", verb: "deploy", order: 30,
		summary: "Build the Worker and put it live",
		targets: []string{"cloudflare"},
		usage:   [][2]string{{"cloudflare", "Build the Worker and deploy it"}},
		notes: `Builds first, so what goes live is what the current source produces.

Cloudflare needs wrangler, which is a Node program — and Cloudflare does not
support the bun runtime. irgo downloads its own Node into ~/.irgo rather than
asking you to install one, and irgo tools remove takes it away again. A working
node already on PATH is used instead.

Credentials:
  CLOUDFLARE_API_TOKEN   required in CI
                         From a terminal, wrangler opens a browser instead.

The Worker's name and any bindings come from wrangler.toml, which is yours
after irgo seeds it.`,
	})
}
