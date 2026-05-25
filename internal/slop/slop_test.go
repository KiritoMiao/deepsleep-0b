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

func TestDefaultPhraseFileIncludesRequestedChineseWords(t *testing.T) {
	t.Parallel()

	entries, err := slop.LoadFile(filepath.Join("..", "..", "data", "slop.json"))
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}

	seen := make(map[string]slop.Entry, len(entries))
	for _, entry := range entries {
		seen[entry.Text] = entry
	}

	words := []string{
		"我会用",
		"最直接",
		"最真相",
		"最不绕弯",
		"最扎心",
		"最硬核",
		"最干脆",
		"最不墨迹",
		"最戳痛点",
		"最不留情面",
		"最一针见血",
		"最开门见山",
		"最单刀直入",
		"最不铺垫",
		"最不客套",
		"最不煽情",
		"最不废话",
		"最不拐弯",
		"最不磨叽",
		"最不装",
		"最不端着",
		"最不啰嗦",
		"最不拖沓",
		"最不委婉",
		"最不掩饰",
		"最不藏着掖着",
		"最直白",
		"最露骨",
		"最实在",
		"最通透",
		"最毒辣",
		"最爽快",
		"最解气",
		"最上头",
		"最够劲",
		"最过瘾",
		"最粗暴",
		"最有效",
		"最狠",
		"最准",
		"最稳",
		"最绝",
		"最顶",
		"最炸",
		"最刚",
		"最烈",
		"最飒",
		"最莽",
		"最冲",
		"最猛",
		"最脆",
		"最亮",
		"最透",
		"最干",
		"最不讲虚的",
		"最不玩套路",
		"最不搞形式",
		"最不整虚头巴脑",
		"最只讲干货",
		"最只说重点",
		"最只给结果",
		"最只聊真相",
		"最只谈核心",
		"最只戳关键",
		"确实是这样的",
	}

	for _, word := range words {
		entry, ok := seen[word]
		if !ok {
			t.Fatalf("expected default phrase file to include %q", word)
		}
		if entry.Lang != slop.LangChinese {
			t.Fatalf("expected %q to be Chinese, got %q", word, entry.Lang)
		}
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
