package api_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"deepsleep.local/deepsleep0b/internal/api"
	"deepsleep.local/deepsleep0b/internal/generator"
	"deepsleep.local/deepsleep0b/internal/slop"
)

type fixedRNG struct {
	floats []float64
}

func (r *fixedRNG) Intn(n int) int { return 0 }

func (r *fixedRNG) Float64() float64 {
	if len(r.floats) == 0 {
		return 0.99
	}
	v := r.floats[0]
	r.floats = r.floats[1:]
	return v
}

func newTestServer(rng generator.Random) http.Handler {
	phrases := []slop.Entry{
		{Text: "Let me think", Lang: slop.LangEnglish, RepeatLimit: 1, Position: slop.PositionFrontOnly},
		{Text: "carefully", Lang: slop.LangEnglish, RepeatLimit: 5, Position: slop.PositionAny},
		{Text: "to conclude", Lang: slop.LangEnglish, RepeatLimit: 1, Position: slop.PositionBackOnly},
		{Text: "让我想想", Lang: slop.LangChinese, RepeatLimit: 1, Position: slop.PositionFrontOnly},
		{Text: "及其", Lang: slop.LangChinese, RepeatLimit: 5, Position: slop.PositionAny},
		{Text: "总而言之", Lang: slop.LangChinese, RepeatLimit: 1, Position: slop.PositionBackOnly},
	}
	return api.NewServer(api.Config{
		Generator: generator.New(phrases, rng),
		IndexHTML: []byte("<!doctype html><title>deepsleep</title>"),
		Domain:    "deepsleep.isclaude.com",
	})
}

func TestModelsListsOnlyAdvertisedModels(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	newTestServer(&fixedRNG{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Object string `json:"object"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Object != "list" || len(body.Data) != 2 || body.Data[0].ID != "deepsleep" || body.Data[1].ID != "deepsleep-0b" {
		t.Fatalf("unexpected models response: %#v", body)
	}
}

func TestOpenAIChatAcceptsAnyModelAndToken(t *testing.T) {
	t.Parallel()

	payload := `{"model":"anything","messages":[{"role":"user","content":"hello there friend"}],"max_tokens":20}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer sk-anything")
	rec := httptest.NewRecorder()
	newTestServer(&fixedRNG{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Object  string `json:"object"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Model != "anything" || body.Choices[0].Message.Role != "assistant" {
		t.Fatalf("unexpected chat response: %#v", body)
	}
	if body.Usage.PromptTokens != 3 || body.Usage.CompletionTokens == 0 || body.Usage.TotalTokens != body.Usage.PromptTokens+body.Usage.CompletionTokens {
		t.Fatalf("bad usage: %#v", body.Usage)
	}
}

func TestOpenAIChatCanReturnBashToolCall(t *testing.T) {
	t.Parallel()

	payload := `{"model":"anything","messages":[{"role":"user","content":"pause"}],"tools":[{"type":"function","function":{"name":"bash","description":"run shell"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	newTestServer(&fixedRNG{floats: []float64{0.05, 0.0}}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Choices[0].FinishReason != "tool_calls" || body.Choices[0].Message.ToolCalls[0].Function.Name != "bash" {
		t.Fatalf("expected bash tool call, got %#v", body)
	}
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(body.Choices[0].Message.ToolCalls[0].Function.Arguments), &args); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(args.Command, "sleep ") {
		t.Fatalf("expected sleep command, got %q", args.Command)
	}
}

func TestOpenAIChatUsesLatestInputAndHalfWeightedHistory(t *testing.T) {
	t.Parallel()

	payload := `{"model":"anything","messages":[{"role":"user","content":"alpha beta gamma delta"},{"role":"assistant","content":"one two three four"},{"role":"user","content":"now answer"}],"max_tokens":40}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	newTestServer(&fixedRNG{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Usage.PromptTokens != 6 {
		t.Fatalf("expected latest user input tokens plus half history tokens, got %d", body.Usage.PromptTokens)
	}
}

func TestOpenAIChatStreamsSSE(t *testing.T) {
	t.Parallel()

	payload := `{"model":"stream-model","stream":true,"messages":[{"role":"user","content":"hello there"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	newTestServer(&fixedRNG{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected event stream content type, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "data: ") || !strings.Contains(rec.Body.String(), "data: [DONE]") {
		t.Fatalf("expected SSE data and done marker, got %s", rec.Body.String())
	}
}

func TestLegacyCompletionEndpoint(t *testing.T) {
	t.Parallel()

	payload := `{"model":"legacy","prompt":"hello completion","max_tokens":20}`
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	newTestServer(&fixedRNG{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"object":"text_completion"`) {
		t.Fatalf("unexpected completion body: %s", rec.Body.String())
	}
}

func TestClaudeMessagesAcceptsAnyModelAndToken(t *testing.T) {
	t.Parallel()

	payload := `{"model":"claude-ish","max_tokens":20,"messages":[{"role":"user","content":"你好世界"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(payload))
	req.Header.Set("x-api-key", "not-real")
	req.Header.Set("anthropic-version", "2023-06-01")
	rec := httptest.NewRecorder()
	newTestServer(&fixedRNG{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Model   string `json:"model"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Type != "message" || body.Role != "assistant" || body.Model != "claude-ish" || !strings.Contains(body.Content[0].Text, "让我想想") {
		t.Fatalf("unexpected claude response: %#v", body)
	}
	if body.Usage.InputTokens != 4 || body.Usage.OutputTokens == 0 {
		t.Fatalf("bad usage: %#v", body.Usage)
	}
}

func TestClaudeMessagesStreamsSSE(t *testing.T) {
	t.Parallel()

	payload := `{"model":"claude-ish","stream":true,"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	newTestServer(&fixedRNG{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	scanner := bufio.NewScanner(bytes.NewReader(rec.Body.Bytes()))
	seenStart := false
	seenStop := false
	for scanner.Scan() {
		line := scanner.Text()
		seenStart = seenStart || strings.Contains(line, "message_start")
		seenStop = seenStop || strings.Contains(line, "message_stop")
	}
	if !seenStart || !seenStop {
		t.Fatalf("expected claude stream lifecycle, got %s", rec.Body.String())
	}
}

func TestClaudeMessagesUsesLatestInputAndHalfWeightedHistory(t *testing.T) {
	t.Parallel()

	payload := `{"model":"claude-ish","messages":[{"role":"user","content":"alpha beta gamma delta"},{"role":"assistant","content":"one two three four"},{"role":"user","content":"now answer"}],"max_tokens":40}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	newTestServer(&fixedRNG{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Usage.InputTokens != 6 {
		t.Fatalf("expected latest user input tokens plus half history tokens, got %d", body.Usage.InputTokens)
	}
}

func TestRootServesFrontendAndHealthz(t *testing.T) {
	t.Parallel()

	server := api.NewServer(api.Config{
		Generator: generator.New([]slop.Entry{{Text: "ok", Lang: slop.LangEnglish, RepeatLimit: 1, Position: slop.PositionAny}}, &fixedRNG{}),
		IndexHTML: []byte(`<!doctype html>
<p>OpenAI endpoint <code>https://DOMAIN/v1/chat/completions</code></p>
<p>Claude endpoint <code>https://DOMAIN/v1/messages</code></p>`),
		Domain: "deepsleep.isclaude.com",
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("root status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "https://deepsleep.isclaude.com/v1/chat/completions") ||
		!strings.Contains(rec.Body.String(), "https://deepsleep.isclaude.com/v1/messages") ||
		strings.Contains(rec.Body.String(), "https://DOMAIN/") {
		t.Fatalf("root should render configured domain, got %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFrontendDisplaysTokenCounts(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("../../web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	for _, want := range []string{
		"Prompt tokens",
		"Completion tokens",
		"Total tokens",
		"usage.prompt_tokens",
		"OpenAI endpoint",
		"https://DOMAIN/v1/chat/completions",
		"Claude endpoint",
		"https://DOMAIN/v1/messages",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("frontend should include %q in token count UI", want)
		}
	}
	messagesIndex := strings.Index(html, `id="messages"`)
	tokenStatsIndex := strings.Index(html, `id="tokenStats"`)
	composerIndex := strings.Index(html, `id="composer"`)
	if messagesIndex == -1 || tokenStatsIndex == -1 || composerIndex == -1 {
		t.Fatalf("frontend should include messages, token stats, and composer sections")
	}
	if !(messagesIndex < tokenStatsIndex && tokenStatsIndex < composerIndex) {
		t.Fatalf("token count should be below chat history and above composer")
	}
}
