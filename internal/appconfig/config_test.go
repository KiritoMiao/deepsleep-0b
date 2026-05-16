package appconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"deepsleep.local/deepsleep0b/internal/appconfig"
)

func TestLoadDomainFromConfigFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"domain":"deepsleep.isclaude.com"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	config, err := appconfig.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	if config.Domain != "deepsleep.isclaude.com" {
		t.Fatalf("expected configured domain, got %q", config.Domain)
	}
}

func TestLoadRejectsInvalidDomain(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"empty":   `{"domain":""}`,
		"scheme":  `{"domain":"https://deepsleep.isclaude.com"}`,
		"slashes": `{"domain":"deepsleep.isclaude.com/v1"}`,
	}
	for name, body := range tests {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := appconfig.LoadFile(path); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
