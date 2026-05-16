package generator_test

import (
	"strings"
	"testing"

	"deepsleep.local/deepsleep0b/internal/generator"
	"deepsleep.local/deepsleep0b/internal/slop"
)

type fixedRNG struct {
	ints   []int
	floats []float64
}

func (r *fixedRNG) Intn(n int) int {
	if n <= 0 {
		panic("invalid Intn bound")
	}
	if len(r.ints) == 0 {
		return 0
	}
	v := r.ints[0]
	r.ints = r.ints[1:]
	if v < 0 {
		v = -v
	}
	return v % n
}

func (r *fixedRNG) Float64() float64 {
	if len(r.floats) == 0 {
		return 0.99
	}
	v := r.floats[0]
	r.floats = r.floats[1:]
	return v
}

func testPhrases() []slop.Entry {
	return []slop.Entry{
		{Text: "Let me think", Lang: slop.LangEnglish, RepeatLimit: 1, Position: slop.PositionFrontOnly},
		{Text: "carefully", Lang: slop.LangEnglish, RepeatLimit: 3, Position: slop.PositionAny},
		{Text: "in this intricate realm", Lang: slop.LangEnglish, RepeatLimit: 2, Position: slop.PositionMiddleOnly},
		{Text: "to conclude", Lang: slop.LangEnglish, RepeatLimit: 1, Position: slop.PositionBackOnly},
		{Text: "让我想想", Lang: slop.LangChinese, RepeatLimit: 1, Position: slop.PositionFrontOnly},
		{Text: "及其", Lang: slop.LangChinese, RepeatLimit: 3, Position: slop.PositionAny},
		{Text: "总而言之", Lang: slop.LangChinese, RepeatLimit: 1, Position: slop.PositionBackOnly},
	}
}

func TestDetectLanguage(t *testing.T) {
	t.Parallel()

	if got := generator.DetectLanguage("please explain this"); got != slop.LangEnglish {
		t.Fatalf("expected english, got %q", got)
	}
	if got := generator.DetectLanguage("请解释 this"); got != slop.LangChinese {
		t.Fatalf("expected chinese, got %q", got)
	}
}

func TestCountWordsUsesEnglishWordsAndChineseCharacters(t *testing.T) {
	t.Parallel()

	tests := map[string]int{
		"hello, steady world":  3,
		"让我稳稳地接住你":             8,
		"sleep 10 秒 please":    4,
		"punctuation... only!": 2,
	}
	for input, want := range tests {
		if got := generator.CountWords(input); got != want {
			t.Fatalf("CountWords(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestGenerateRespectsPositionsRepeatLimitsAndMaxTokens(t *testing.T) {
	t.Parallel()

	g := generator.New(testPhrases(), &fixedRNG{})
	result := g.Generate(generator.Request{
		Input:     "one two three four five six seven eight nine ten eleven twelve",
		MaxTokens: 10,
	})

	if result.Language != slop.LangEnglish {
		t.Fatalf("expected english, got %q", result.Language)
	}
	if result.InputTokens != 12 {
		t.Fatalf("expected 12 input tokens, got %d", result.InputTokens)
	}
	if result.OutputTokens > 10 {
		t.Fatalf("expected output to be capped at 10 words, got %d text=%q", result.OutputTokens, result.Text)
	}
	if !strings.HasPrefix(result.Text, "Let me think") {
		t.Fatalf("expected front phrase at start, got %q", result.Text)
	}
	if !strings.HasSuffix(result.Text, "to conclude") {
		t.Fatalf("expected back phrase at end, got %q", result.Text)
	}
	if strings.Count(result.Text, "Let me think") > 1 {
		t.Fatalf("front phrase exceeded repeat limit: %q", result.Text)
	}
}

func TestThinkLevelMultiplierIncreasesOutputLength(t *testing.T) {
	t.Parallel()

	low := generator.New(testPhrases(), &fixedRNG{}).Generate(generator.Request{
		Input:      "one two three four five six seven eight nine ten",
		ThinkLevel: "low",
		MaxTokens:  50,
	})
	high := generator.New(testPhrases(), &fixedRNG{}).Generate(generator.Request{
		Input:      "one two three four five six seven eight nine ten",
		ThinkLevel: "high",
		MaxTokens:  50,
	})

	if high.OutputTokens <= low.OutputTokens {
		t.Fatalf("expected high think level output > low, low=%d high=%d", low.OutputTokens, high.OutputTokens)
	}
}

func TestHistoryContributesHalfWeightedInputTokens(t *testing.T) {
	t.Parallel()

	result := generator.New(testPhrases(), &fixedRNG{}).Generate(generator.Request{
		Input:     "one two three four",
		History:   "alpha beta gamma delta epsilon zeta",
		MaxTokens: 50,
	})

	if result.InputTokens != 7 {
		t.Fatalf("expected current input plus half-weight history tokens, got %d", result.InputTokens)
	}
}

func TestInputSeedMakesGenerationDeterministicPerConversation(t *testing.T) {
	t.Parallel()

	req := generator.Request{
		Input:        "alpha beta gamma delta epsilon",
		History:      "previous answer context",
		UseInputSeed: true,
		MaxTokens:    30,
	}
	first := generator.New(testPhrases(), &fixedRNG{ints: []int{3, 2, 1}}).Generate(req)
	second := generator.New(testPhrases(), &fixedRNG{ints: []int{99, 88, 77}}).Generate(req)

	if first.Text != second.Text {
		t.Fatalf("expected same input/history seed to produce same text, first=%q second=%q", first.Text, second.Text)
	}
}

func TestMultiplierWeightsPhraseSelection(t *testing.T) {
	t.Parallel()

	phrases := []slop.Entry{
		{Text: "rare front", Lang: slop.LangEnglish, RepeatLimit: 1, Position: slop.PositionFrontOnly, Multiplier: 1},
		{Text: "Sleeping...", Lang: slop.LangEnglish, RepeatLimit: 1, Position: slop.PositionFrontOnly, Multiplier: 10},
		{Text: "carefully", Lang: slop.LangEnglish, RepeatLimit: 4, Position: slop.PositionAny, Multiplier: 1},
	}
	result := generator.New(phrases, &fixedRNG{ints: []int{10}}).Generate(generator.Request{
		Input:     "one two three four five six seven eight",
		MaxTokens: 20,
	})

	if !strings.HasPrefix(result.Text, "Sleeping...") {
		t.Fatalf("expected weighted sleeping phrase to be selected, got %q", result.Text)
	}
}

func TestSharedLanguagePhraseCanAppearInEnglishAndChinese(t *testing.T) {
	t.Parallel()

	phrases := []slop.Entry{
		{Text: "Shared...", Lang: slop.LangBoth, RepeatLimit: 1, Position: slop.PositionFrontOnly, Multiplier: 1},
		{Text: "carefully", Lang: slop.LangEnglish, RepeatLimit: 4, Position: slop.PositionAny, Multiplier: 1},
		{Text: "及其", Lang: slop.LangChinese, RepeatLimit: 4, Position: slop.PositionAny, Multiplier: 1},
	}
	english := generator.New(phrases, &fixedRNG{}).Generate(generator.Request{Input: "hello world", MaxTokens: 20})
	chinese := generator.New(phrases, &fixedRNG{}).Generate(generator.Request{Input: "你好世界", MaxTokens: 20})

	if !strings.HasPrefix(english.Text, "Shared...") {
		t.Fatalf("expected shared phrase in english output, got %q", english.Text)
	}
	if !strings.HasPrefix(chinese.Text, "Shared...") {
		t.Fatalf("expected shared phrase in chinese output, got %q", chinese.Text)
	}
}

func TestGenerateChineseUsesChinesePhraseBank(t *testing.T) {
	t.Parallel()

	g := generator.New(testPhrases(), &fixedRNG{})
	result := g.Generate(generator.Request{Input: "请解释一下这个问题", MaxTokens: 20})

	if result.Language != slop.LangChinese {
		t.Fatalf("expected chinese, got %q", result.Language)
	}
	if !strings.Contains(result.Text, "让我想想") || !strings.Contains(result.Text, "及其") {
		t.Fatalf("expected chinese phrases, got %q", result.Text)
	}
}

func TestBashToolCallCanBeGenerated(t *testing.T) {
	t.Parallel()

	g := generator.New(testPhrases(), &fixedRNG{floats: []float64{0.05, 0.5}})
	result := g.Generate(generator.Request{
		Input: "run something slowly",
		Tools: []generator.Tool{{Name: "shell", Bash: true}},
	})

	if result.ToolCall == nil {
		t.Fatal("expected tool call")
	}
	if result.ToolCall.Name != "shell" {
		t.Fatalf("expected submitted tool name, got %q", result.ToolCall.Name)
	}
	if result.ToolCall.Command != "sleep 5.0" {
		t.Fatalf("expected deterministic sleep command, got %q", result.ToolCall.Command)
	}
}
