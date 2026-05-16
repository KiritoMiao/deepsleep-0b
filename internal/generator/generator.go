package generator

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"regexp"
	"strings"
	"unicode"

	"deepsleep.local/deepsleep0b/internal/slop"
)

type Random interface {
	Intn(n int) int
	Float64() float64
}

type Tool struct {
	Name string
	Bash bool
}

type ToolCall struct {
	ID           string
	Name         string
	Command      string
	SleepSeconds float64
}

type Request struct {
	Input        string
	History      string
	MaxTokens    int
	ThinkLevel   string
	Tools        []Tool
	UseInputSeed bool
	RandomSeed   int64
}

type Result struct {
	Text         string
	InputTokens  int
	OutputTokens int
	Language     slop.Lang
	ToolCall     *ToolCall
}

type Generator struct {
	phrases []slop.Entry
	rng     Random
}

func New(phrases []slop.Entry, rng Random) *Generator {
	return &Generator{phrases: phrases, rng: rng}
}

func DetectLanguage(input string) slop.Lang {
	for _, r := range input {
		if unicode.Is(unicode.Han, r) {
			return slop.LangChinese
		}
	}
	return slop.LangEnglish
}

func CountWords(input string) int {
	count := 0
	inASCIIWord := false

	for _, r := range input {
		switch {
		case unicode.Is(unicode.Han, r):
			if inASCIIWord {
				inASCIIWord = false
			}
			count++
		case isWordRune(r):
			if !inASCIIWord {
				count++
				inASCIIWord = true
			}
		default:
			inASCIIWord = false
		}
	}
	return count
}

func (g *Generator) Generate(req Request) Result {
	lang := DetectLanguage(req.Input)
	if strings.TrimSpace(req.Input) == "" && strings.TrimSpace(req.History) != "" {
		lang = DetectLanguage(req.History)
	}
	rng := g.requestRNG(req)
	inputTokens := WeightedInputTokens(req.Input, req.History)
	tools := bashTools(req.Tools)
	if len(tools) > 0 && randomFloat(g.rng) < 0.10 {
		sleepSeconds := 0.1 + randomFloat(g.rng)*9.9
		sleepSeconds = math.Floor(sleepSeconds*10) / 10
		return Result{
			InputTokens: inputTokens,
			Language:    lang,
			ToolCall: &ToolCall{
				ID:           fmt.Sprintf("call_%06d", randomIntn(g.rng, 1000000)),
				Name:         tools[randomIntn(g.rng, len(tools))].Name,
				Command:      fmt.Sprintf("sleep %.1f", sleepSeconds),
				SleepSeconds: sleepSeconds,
			},
		}
	}

	target := targetWords(inputTokens, req.MaxTokens, req.ThinkLevel)
	words := g.generateWords(lang, target, rng)
	text := joinWords(lang, words)
	return Result{
		Text:         text,
		InputTokens:  inputTokens,
		OutputTokens: CountWords(text),
		Language:     lang,
	}
}

func WeightedInputTokens(input string, history string) int {
	current := CountWords(input)
	historical := CountWords(history)
	if historical == 0 {
		return current
	}
	return current + int(math.Ceil(float64(historical)*0.5))
}

func (g *Generator) requestRNG(req Request) Random {
	if !req.UseInputSeed {
		return g.rng
	}
	return rand.New(rand.NewSource(conversationSeed(req.Input, req.History, req.RandomSeed)))
}

func conversationSeed(input string, history string, randomSeed int64) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(input))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(history))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(fmt.Sprintf("%d", randomSeed)))
	return int64(hash.Sum64())
}

func (g *Generator) generateWords(lang slop.Lang, target int, rng Random) []string {
	if target <= 0 {
		return nil
	}

	var front []slop.Entry
	var middle []slop.Entry
	var back []slop.Entry
	for _, phrase := range g.phrases {
		if phrase.Lang != lang && phrase.Lang != slop.LangBoth {
			continue
		}
		switch phrase.Position {
		case slop.PositionFrontOnly:
			front = append(front, phrase)
		case slop.PositionBackOnly:
			back = append(back, phrase)
		case slop.PositionAny, slop.PositionMiddleOnly:
			middle = append(middle, phrase)
		}
	}

	used := map[string]int{}
	var out []string
	current := 0
	backPhrase, backWords := pickFittingBack(back, target, used)

	if phrase, n, ok := pickFitting(front, target-backWords, used, rng); ok {
		out = append(out, phrase.Text)
		used[phrase.Text]++
		current += n
	}

	for current < target {
		remaining := target - current - backWords
		if remaining <= 0 {
			break
		}
		phrase, n, ok := pickFitting(middle, remaining, used, rng)
		if !ok {
			break
		}
		out = append(out, phrase.Text)
		used[phrase.Text]++
		current += n
	}

	if backPhrase.Text != "" && current+backWords <= target {
		out = append(out, backPhrase.Text)
		used[backPhrase.Text]++
	}

	if len(out) == 0 {
		for _, phrase := range g.phrases {
			if (phrase.Lang == lang || phrase.Lang == slop.LangBoth) && CountWords(phrase.Text) <= target {
				return []string{phrase.Text}
			}
		}
	}
	return out
}

func pickFitting(candidates []slop.Entry, maxWords int, used map[string]int, rng Random) (slop.Entry, int, bool) {
	if maxWords <= 0 || len(candidates) == 0 {
		return slop.Entry{}, 0, false
	}
	fitting := make([]slop.Entry, 0, len(candidates))
	totalWeight := 0
	for _, phrase := range candidates {
		if exhausted(phrase, used) {
			continue
		}
		words := CountWords(phrase.Text)
		if words <= maxWords {
			weight := phrase.Multiplier
			if weight <= 0 {
				weight = 1
			}
			totalWeight += weight
			fitting = append(fitting, phrase)
		}
	}
	if totalWeight <= 0 {
		return slop.Entry{}, 0, false
	}
	pick := randomIntn(rng, totalWeight)
	for _, phrase := range fitting {
		weight := phrase.Multiplier
		if weight <= 0 {
			weight = 1
		}
		if pick < weight {
			return phrase, CountWords(phrase.Text), true
		}
		pick -= weight
	}
	return slop.Entry{}, 0, false
}

func pickFittingBack(candidates []slop.Entry, target int, used map[string]int) (slop.Entry, int) {
	for _, phrase := range candidates {
		if exhausted(phrase, used) {
			continue
		}
		words := CountWords(phrase.Text)
		if words <= target {
			return phrase, words
		}
	}
	return slop.Entry{}, 0
}

func exhausted(phrase slop.Entry, used map[string]int) bool {
	return phrase.RepeatLimit > 0 && used[phrase.Text] >= phrase.RepeatLimit
}

func targetWords(inputTokens int, maxTokens int, thinkLevel string) int {
	if inputTokens <= 0 {
		inputTokens = 1
	}
	target := int(math.Ceil(float64(inputTokens) * 1.2 * thinkMultiplier(thinkLevel)))
	if target < 8 {
		target = 8
	}
	if maxTokens > 0 && target > maxTokens {
		target = maxTokens
	}
	if target < 1 {
		target = 1
	}
	return target
}

func thinkMultiplier(level string) float64 {
	normalized := strings.ToLower(strings.TrimSpace(level))
	switch normalized {
	case "none", "off", "disabled":
		return 0.6
	case "low", "minimal":
		return 0.8
	case "high":
		return 1.6
	case "max", "maximum", "deep":
		return 2.4
	default:
		return 1.0
	}
}

func bashTools(tools []Tool) []Tool {
	var out []Tool
	for _, tool := range tools {
		if tool.Bash {
			if strings.TrimSpace(tool.Name) == "" {
				tool.Name = "bash"
			}
			out = append(out, tool)
		}
	}
	return out
}

func joinWords(lang slop.Lang, phrases []string) string {
	if lang == slop.LangChinese {
		return strings.Join(phrases, "，")
	}
	return strings.Join(phrases, " ")
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func randomIntn(rng Random, n int) int {
	if rng == nil {
		return 0
	}
	return rng.Intn(n)
}

func randomFloat(rng Random) float64 {
	if rng == nil {
		return 0.99
	}
	return rng.Float64()
}

var whitespaceRE = regexp.MustCompile(`\s+`)

func NormalizePrompt(parts ...string) string {
	var joined []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			joined = append(joined, part)
		}
	}
	return whitespaceRE.ReplaceAllString(strings.Join(joined, "\n"), " ")
}
