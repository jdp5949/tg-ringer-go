package main

import (
	"os"
	"testing"
)

func TestConfigRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TG_RINGER_HOME", dir)
	for _, k := range []string{"TG_API_ID", "TG_API_HASH", "TG_TARGET"} {
		os.Unsetenv(k)
	}

	if err := saveConfig(map[string]string{
		"TG_API_ID":   "111",
		"TG_API_HASH": "abc123def456",
		"TG_TARGET":   "+15551234567",
	}); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	// config file must be chmod 600
	info, err := os.Stat(configFile())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config not private: %v", info.Mode().Perm())
	}

	for _, k := range []string{"TG_API_ID", "TG_API_HASH", "TG_TARGET"} {
		os.Unsetenv(k)
	}
	loadConfig()
	if got := os.Getenv("TG_API_ID"); got != "111" {
		t.Errorf("TG_API_ID = %q, want 111", got)
	}
	if got := os.Getenv("TG_API_HASH"); got != "abc123def456" {
		t.Errorf("TG_API_HASH = %q", got)
	}
}

func TestTargetFallback(t *testing.T) {
	t.Setenv("TG_TARGET", "+19998887777")
	got, err := target("")
	if err != nil || got != "+19998887777" {
		t.Fatalf("target fallback = %q, %v", got, err)
	}
	got, err = target("@explicit")
	if err != nil || got != "@explicit" {
		t.Fatalf("target explicit = %q, %v", got, err)
	}
}
