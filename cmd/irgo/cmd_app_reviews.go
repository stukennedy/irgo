package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ---------------------------------------------------------------------------
// irgo app reviews — app store review monitoring (and responding).
//
// Apple (iOS + Mac App Store): official App Store Connect API — ES256 JWT auth
// with a .p8 key. Lists reviews and posts responses (reviews_apple.go).
// Android: Google Play Developer API with a service-account JWT. Lists reviews
// and replies (reviews_play.go).
//
// Config lives in irgo.package.toml under [reviews]:
//
//	[reviews]
//	ios_app_id = ""               # numeric iOS App Store id
//	mac_app_id = ""               # numeric Mac App Store id
//	ios_key_id = ""               # App Store Connect API key id
//	ios_issuer_id = ""            # App Store Connect API issuer id
//	ios_private_key = ""          # path to the .p8 private key
//	android_package = ""          # e.g. "com.example.myapp"
//	android_service_account = ""  # path to a Play service-account JSON
//
// `--new` prints only reviews newer than the last check (state in .irgo/).
// ---------------------------------------------------------------------------

const reviewsStateFile = ".irgo/reviews-state.json"

type reviewsState struct {
	IOS     string `json:"ios_last_seen,omitempty"`     // createdDate of newest iOS review seen
	Mac     string `json:"mac_last_seen,omitempty"`     // createdDate of newest Mac review seen
	Android int64  `json:"android_last_seen,omitempty"` // epoch seconds of newest Play review seen
}

func reviewsUsage() string {
	return `Usage:
  irgo app reviews <ios|mac|android> [--limit N] [--new]
  irgo app reviews <ios|mac|android> --reply <reviewId> --text "reply text"

Flags:
  --limit N     how many recent reviews to show (default 10)
  --new         only show reviews newer than the last check (monitoring)
  --reply ID    reply to a review (Apple + Play) — must also pass --text
  --text "..."  the reply text

Config (irgo.package.toml → [reviews]): ios_app_id, mac_app_id, ios_key_id,
ios_issuer_id, ios_private_key, android_package, android_service_account.
` + "`irgo app package setup`" + ` explains where to get each.

Apple reviews use the official App Store Connect API (key needs Customer
Reviews access); Android uses the Play Developer API (service account).`
}

func reviewsCommand(args []string) error {
	if len(args) < 1 {
		fmt.Println(reviewsUsage())
		return fmt.Errorf("reviews requires a store: ios, mac or android")
	}
	store := args[0]
	limit := 10
	onlyNew := false
	replyTo, replyText := "", ""
	for i := 1; i < len(args); i++ {
		next := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch args[i] {
		case "--limit":
			if v := next(); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					limit = n
				}
			}
		case "--new":
			onlyNew = true
		case "--reply":
			replyTo = next()
		case "--text":
			replyText = next()
		default:
			return fmt.Errorf("unknown flag: %s", args[i])
		}
	}
	switch store {
	case "ios", "mac":
		return reviewsApple(store, limit, onlyNew, replyTo, replyText)
	case "android":
		return reviewsAndroid(limit, onlyNew, replyTo, replyText)
	default:
		return fmt.Errorf("unknown store: %s (use ios, mac or android)", store)
	}
}

// ---------------------------------------------------------------------------
// state + http helpers (shared by reviews_apple.go and reviews_play.go)
// ---------------------------------------------------------------------------

func loadReviewsState() reviewsState {
	var st reviewsState
	if data, err := os.ReadFile(reviewsStateFile); err == nil {
		_ = json.Unmarshal(data, &st)
	}
	return st
}

func saveReviewsState(st reviewsState) error {
	if err := os.MkdirAll(filepath.Dir(reviewsStateFile), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(st, "", "  ")
	return os.WriteFile(reviewsStateFile, data, 0o644)
}

func httpGet(u string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, u)
	}
	return body, nil
}

func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func shortDate(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	return t.Format("2006-01-02")
}

func epochToRFC3339(sec int64) string {
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

func init() {
	register(command{
		noun: "app", verb: "reviews", order: 60,
		summary: "Monitor store reviews",
		targets: []string{"ios", "mac", "android"},
		usage: [][2]string{
			{"<store>", "Fetch reviews"},
			{"ios --new", "Only ones you have not seen"},
			{"ios --reply <id> --text \"...\"", "Reply to one"},
		},
		flags: [][2]string{
			{"--limit <n>", "How many to fetch"},
			{"--new", "Only ones you have not seen"},
			{"--reply <id>", "Reply to one review"},
			{"--text \"...\"", "The reply body"},
		},
		notes: `Needs the store credentials in irgo.package.toml under [reviews] — see
irgo project config.`,
	})
}
