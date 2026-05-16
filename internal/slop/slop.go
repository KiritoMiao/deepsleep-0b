package slop

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type Lang string

const (
	LangEnglish Lang = "en"
	LangChinese Lang = "zh"
	LangBoth    Lang = "both"
)

type Position string

const (
	PositionAny        Position = "any"
	PositionFrontOnly  Position = "front_only"
	PositionMiddleOnly Position = "middle_only"
	PositionBackOnly   Position = "back_only"
)

type Entry struct {
	Text        string   `json:"text"`
	Lang        Lang     `json:"lang"`
	RepeatLimit int      `json:"repeat_limit"`
	Position    Position `json:"position"`
	Multiplier  int      `json:"multiplier,omitempty"`
}

func (e *Entry) UnmarshalJSON(data []byte) error {
	var raw struct {
		Text        string   `json:"text"`
		Lang        Lang     `json:"lang"`
		RepeatLimit int      `json:"repeat_limit"`
		Position    Position `json:"position"`
		Multiplier  *int     `json:"multiplier"`
		Multiplex   *int     `json:"multiplex"`
		Mutiplex    *int     `json:"mutiplex"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return err
	}

	multiplier := 1
	switch {
	case raw.Multiplier != nil:
		multiplier = *raw.Multiplier
	case raw.Mutiplex != nil:
		multiplier = *raw.Mutiplex
	case raw.Multiplex != nil:
		multiplier = *raw.Multiplex
	}

	*e = Entry{
		Text:        raw.Text,
		Lang:        raw.Lang,
		RepeatLimit: raw.RepeatLimit,
		Position:    raw.Position,
		Multiplier:  multiplier,
	}
	return nil
}

func LoadFile(path string) ([]Entry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("slop: open %s: %w", path, err)
	}
	defer file.Close()

	entries, err := Load(file)
	if err != nil {
		return nil, fmt.Errorf("slop: %s: %w", path, err)
	}
	return entries, nil
}

func Load(r io.Reader) ([]Entry, error) {
	var entries []Entry
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("slop: decode: %w", err)
	}
	if err := Validate(entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func Validate(entries []Entry) error {
	if len(entries) == 0 {
		return fmt.Errorf("slop: phrase list is empty")
	}
	for i, entry := range entries {
		if strings.TrimSpace(entry.Text) == "" {
			return fmt.Errorf("slop: entry %d has empty text", i)
		}
		switch entry.Lang {
		case LangEnglish, LangChinese, LangBoth:
		default:
			return fmt.Errorf("slop: entry %d has unsupported lang %q", i, entry.Lang)
		}
		if entry.RepeatLimit < 0 {
			return fmt.Errorf("slop: entry %d has negative repeat_limit", i)
		}
		if entry.Multiplier <= 0 {
			return fmt.Errorf("slop: entry %d has non-positive multiplier", i)
		}
		switch entry.Position {
		case PositionAny, PositionFrontOnly, PositionMiddleOnly, PositionBackOnly:
		default:
			return fmt.Errorf("slop: entry %d has unsupported position %q", i, entry.Position)
		}
	}
	return nil
}
