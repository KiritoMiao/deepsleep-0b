package slop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepsleep.local/deepsleep0b/internal/slop"
)

func TestLoadValidatesPhraseFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "slop.json")
	body := `[
		{"text":"Let me think...","lang":"en","repeat_limit":2,"position":"front_only","multiplier":3},
		{"text":"让我想想...","lang":"zh","repeat_limit":1,"position":"any"},
		{"text":"Hmm...","lang":"both","repeat_limit":1,"position":"front_only","mutiplex":2}
	]`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := slop.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Text != "Let me think..." || entries[0].Lang != slop.LangEnglish || entries[0].Position != slop.PositionFrontOnly || entries[0].Multiplier != 3 {
		t.Fatalf("unexpected first entry: %#v", entries[0])
	}
	if entries[2].Lang != slop.LangBoth || entries[2].Multiplier != 2 {
		t.Fatalf("expected shared-language mutiplex alias entry, got %#v", entries[2])
	}
}

func TestLoadRejectsInvalidEntries(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"bad lang":       `[{"text":"x","lang":"jp","repeat_limit":1,"position":"any"}]`,
		"bad repeat":     `[{"text":"x","lang":"en","repeat_limit":-1,"position":"any"}]`,
		"bad multiplier": `[{"text":"x","lang":"en","repeat_limit":1,"position":"any","multiplier":0}]`,
		"bad position":   `[{"text":"x","lang":"en","repeat_limit":1,"position":"sideways"}]`,
		"empty text":     `[{"text":"","lang":"en","repeat_limit":1,"position":"any"}]`,
		"empty document": `[]`,
	}

	for name, body := range tests {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "slop.json")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}

			_, err := slop.LoadFile(path)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), "slop") {
				t.Fatalf("expected contextual error, got %v", err)
			}
		})
	}
}
