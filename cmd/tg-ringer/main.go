// Command tg-ringer rings and messages Telegram users from your own account.
//
// Usage:
//
//	tg-ringer login                 interactive setup + sign in
//	tg-ringer call  TARGET [secs]   ring a user/number, then hang up
//	tg-ringer msg   TARGET TEXT...  send a direct message
//	tg-ringer whoami                show the logged-in account
//	tg-ringer config                show current config (api_hash masked)
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"

	"github.com/jdp5949/tg-ringer-go/ringer"
)

func configDir() string {
	if v := os.Getenv("TG_RINGER_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "tg-ringer-go")
}

func configFile() string { return filepath.Join(configDir(), "config") }

// loadConfig reads KEY=VALUE lines into the environment (without overriding).
func loadConfig() {
	data, err := os.ReadFile(configFile())
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if _, ok := os.LookupEnv(k); !ok {
			_ = os.Setenv(k, v)
		}
	}
}

func saveConfig(values map[string]string) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	existing := map[string]string{}
	if data, err := os.ReadFile(configFile()); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "=") && !strings.HasPrefix(strings.TrimSpace(line), "#") {
				k, v, _ := strings.Cut(line, "=")
				existing[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}
	for k, v := range values {
		if v != "" {
			existing[k] = v
		}
	}
	var b strings.Builder
	b.WriteString("# tg-ringer config — keep private (contains api_hash)\n")
	for k, v := range existing {
		fmt.Fprintf(&b, "%s=%s\n", k, v)
	}
	return os.WriteFile(configFile(), []byte(b.String()), 0o600)
}

func prompt(label string) string {
	fmt.Print(label)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}

func sessionFile() string {
	if v := os.Getenv("TG_SESSION"); v != "" {
		return v
	}
	_ = os.MkdirAll(configDir(), 0o700)
	return filepath.Join(configDir(), "userbot.session")
}

func cfg() (ringer.Config, error) {
	loadConfig()
	idStr := os.Getenv("TG_API_ID")
	hash := os.Getenv("TG_API_HASH")
	if idStr == "" || hash == "" {
		return ringer.Config{}, errors.New("not configured — run `tg-ringer login`")
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return ringer.Config{}, fmt.Errorf("invalid TG_API_ID: %w", err)
	}
	return ringer.Config{APIID: id, APIHash: hash, SessionFile: sessionFile()}, nil
}

func interactiveSetup() error {
	fmt.Println("tg-ringer setup — get api_id/api_hash at https://my.telegram.org")
	id := prompt("api_id  : ")
	hash := prompt("api_hash: ")
	target := prompt("default target (optional, e.g. +15551234567): ")
	if id == "" || hash == "" {
		return errors.New("api_id and api_hash are required")
	}
	vals := map[string]string{"TG_API_ID": id, "TG_API_HASH": hash}
	if target != "" {
		vals["TG_TARGET"] = target
	}
	if err := saveConfig(vals); err != nil {
		return err
	}
	loadConfig()
	fmt.Printf("Saved to %s\n", configFile())
	return nil
}

// termAuth implements auth.UserAuthenticator via stdin prompts.
type termAuth struct{}

func (termAuth) Phone(_ context.Context) (string, error) {
	return prompt("phone (+E164): "), nil
}
func (termAuth) Password(_ context.Context) (string, error) {
	return prompt("2FA password: "), nil
}
func (termAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	return prompt("login code (from Telegram app): "), nil
}
func (termAuth) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error {
	return nil
}
func (termAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("sign up is not supported; use an existing account")
}

func target(arg string) (string, error) {
	if arg != "" {
		return arg, nil
	}
	if t := os.Getenv("TG_TARGET"); t != "" {
		return t, nil
	}
	return "", errors.New("no target: pass one or set TG_TARGET")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "login":
		loadConfig()
		if os.Getenv("TG_API_ID") == "" || os.Getenv("TG_API_HASH") == "" {
			if err = interactiveSetup(); err != nil {
				break
			}
		}
		var c ringer.Config
		if c, err = cfg(); err != nil {
			break
		}
		if err = ringer.Login(ctx, c, termAuth{}); err != nil {
			break
		}
		err = ringer.Run(ctx, c, func(ctx context.Context, cl *ringer.Client) error {
			me, e := cl.Self(ctx)
			if e != nil {
				return e
			}
			fmt.Printf("Logged in as %s (id %d, @%s)\n", me.FirstName, me.ID, me.Username)
			return nil
		})
	case "init":
		err = interactiveSetup()
	case "config":
		loadConfig()
		showConfig()
	case "call":
		var c ringer.Config
		if c, err = cfg(); err != nil {
			break
		}
		tgt := ""
		if len(os.Args) > 2 {
			tgt = os.Args[2]
		}
		if tgt, err = target(tgt); err != nil {
			break
		}
		secs := 20
		if v := os.Getenv("RING_SECONDS"); v != "" {
			if n, e := strconv.Atoi(v); e == nil {
				secs = n
			}
		}
		if len(os.Args) > 3 {
			if n, e := strconv.Atoi(os.Args[3]); e == nil {
				secs = n
			}
		}
		err = ringer.Run(ctx, c, func(ctx context.Context, cl *ringer.Client) error {
			fmt.Printf("ringing %s for %ds ...\n", tgt, secs)
			id, e := cl.Ring(ctx, tgt, secs)
			if e != nil {
				return e
			}
			fmt.Printf("done (call id %d)\n", id)
			return nil
		})
	case "msg":
		var c ringer.Config
		if c, err = cfg(); err != nil {
			break
		}
		if len(os.Args) < 4 {
			err = errors.New("usage: tg-ringer msg TARGET TEXT...")
			break
		}
		tgt := os.Args[2]
		text := strings.Join(os.Args[3:], " ")
		err = ringer.Run(ctx, c, func(ctx context.Context, cl *ringer.Client) error {
			if e := cl.Message(ctx, tgt, text); e != nil {
				return e
			}
			fmt.Println("sent")
			return nil
		})
	case "whoami":
		var c ringer.Config
		if c, err = cfg(); err != nil {
			break
		}
		err = ringer.Run(ctx, c, func(ctx context.Context, cl *ringer.Client) error {
			me, e := cl.Self(ctx)
			if e != nil {
				return e
			}
			fmt.Printf("%s (id %d, @%s)\n", me.FirstName, me.ID, me.Username)
			return nil
		})
	case "-h", "--help", "help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func showConfig() {
	hash := os.Getenv("TG_API_HASH")
	masked := "(unset)"
	if len(hash) > 8 {
		masked = hash[:4] + "…" + hash[len(hash)-4:]
	}
	exists := "none"
	if _, err := os.Stat(configFile()); err == nil {
		exists = "exists"
	}
	fmt.Printf("config file : %s (%s)\n", configFile(), exists)
	fmt.Printf("TG_API_ID   : %s\n", orUnset(os.Getenv("TG_API_ID")))
	fmt.Printf("TG_API_HASH : %s\n", masked)
	fmt.Printf("TG_TARGET   : %s\n", orUnset(os.Getenv("TG_TARGET")))
	fmt.Printf("TG_SESSION  : %s\n", sessionFile())
}

func orUnset(s string) string {
	if s == "" {
		return "(unset)"
	}
	return s
}

func usage() {
	fmt.Println(`tg-ringer — ring/message Telegram users from your own account

  tg-ringer login                interactive setup + sign in
  tg-ringer call  TARGET [secs]  ring a user/number, then hang up
  tg-ringer msg   TARGET TEXT...  send a direct message
  tg-ringer whoami               show the logged-in account
  tg-ringer config               show current config
  tg-ringer init                 (re)configure credentials

TARGET: @username, numeric id, or +E164 phone number.`)
}
