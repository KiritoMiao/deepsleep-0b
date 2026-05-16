package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"deepsleep.local/deepsleep0b/internal/generator"
)

type Config struct {
	Generator *generator.Generator
	IndexHTML []byte
	Domain    string
}

type Server struct {
	generator *generator.Generator
	indexHTML []byte
	domain    string
	mux       *http.ServeMux
}

func NewServer(config Config) http.Handler {
	server := &Server{
		generator: config.Generator,
		indexHTML: config.IndexHTML,
		domain:    config.Domain,
		mux:       http.NewServeMux(),
	}
	server.routes()
	return cors(server.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/v1/models", s.handleModels)
	s.mux.HandleFunc("/v1/chat/completions", s.handleOpenAIChat)
	s.mux.HandleFunc("/v1/completions", s.handleOpenAICompletion)
	s.mux.HandleFunc("/v1/messages", s.handleClaudeMessages)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "authorization, content-type, x-api-key, anthropic-version, x-think-level")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(renderIndex(s.indexHTML, s.domain))
}

func renderIndex(indexHTML []byte, domain string) []byte {
	if strings.TrimSpace(domain) == "" {
		return indexHTML
	}
	return []byte(strings.ReplaceAll(string(indexHTML), "https://DOMAIN", "https://"+domain))
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data": []map[string]any{
			{"id": "deepsleep", "object": "model", "created": 1778935891, "owned_by": "deepsleep"},
			{"id": "deepsleep-0b", "object": "model", "created": 1778935891, "owned_by": "deepsleep"},
		},
	})
}

type openAIChatRequest struct {
	Model           string          `json:"model"`
	Messages        []openAIMessage `json:"messages"`
	MaxTokens       int             `json:"max_tokens"`
	Stream          bool            `json:"stream"`
	Tools           []openAITool    `json:"tools"`
	ThinkLevel      string          `json:"think_level"`
	ReasoningEffort string          `json:"reasoning_effort"`
	Thinking        any             `json:"thinking"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"function"`
	Name string `json:"name"`
}

type openAICompletionRequest struct {
	Model           string `json:"model"`
	Prompt          any    `json:"prompt"`
	MaxTokens       int    `json:"max_tokens"`
	Stream          bool   `json:"stream"`
	ThinkLevel      string `json:"think_level"`
	ReasoningEffort string `json:"reasoning_effort"`
	Thinking        any    `json:"thinking"`
}

func (s *Server) handleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req openAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	conversation := openAIConversationText(req.Messages)
	result := s.generator.Generate(generator.Request{
		Input:        conversation.Input,
		History:      conversation.History,
		MaxTokens:    req.MaxTokens,
		ThinkLevel:   selectThinkLevel(r, req.ThinkLevel, req.ReasoningEffort, req.Thinking),
		Tools:        openAITools(req.Tools),
		UseInputSeed: true,
		RandomSeed:   time.Now().UnixNano(),
	})
	if req.Stream {
		s.streamOpenAIChat(w, req.Model, result)
		return
	}
	writeJSON(w, http.StatusOK, openAIChatResponse(req.Model, result))
}

func (s *Server) handleOpenAICompletion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req openAICompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	prompt := textFromAny(req.Prompt)
	result := s.generator.Generate(generator.Request{
		Input:        prompt,
		MaxTokens:    req.MaxTokens,
		ThinkLevel:   selectThinkLevel(r, req.ThinkLevel, req.ReasoningEffort, req.Thinking),
		UseInputSeed: true,
		RandomSeed:   time.Now().UnixNano(),
	})
	if req.Stream {
		s.streamOpenAICompletion(w, req.Model, result)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      newID("cmpl"),
		"object":  "text_completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []map[string]any{{
			"text":          result.Text,
			"index":         0,
			"logprobs":      nil,
			"finish_reason": "stop",
		}},
		"usage": openAIUsage(result),
	})
}

func openAIChatResponse(model string, result generator.Result) map[string]any {
	message := map[string]any{"role": "assistant", "content": result.Text}
	finishReason := "stop"
	if result.ToolCall != nil {
		finishReason = "tool_calls"
		message["content"] = nil
		message["tool_calls"] = []map[string]any{openAIToolCall(result.ToolCall)}
	}
	return map[string]any{
		"id":      newID("chatcmpl"),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       message,
			"finish_reason": finishReason,
		}},
		"usage": openAIUsage(result),
	}
}

func openAIToolCall(call *generator.ToolCall) map[string]any {
	args, _ := json.Marshal(map[string]string{"command": call.Command})
	return map[string]any{
		"id":   call.ID,
		"type": "function",
		"function": map[string]any{
			"name":      call.Name,
			"arguments": string(args),
		},
	}
}

func openAIUsage(result generator.Result) map[string]int {
	return map[string]int{
		"prompt_tokens":     result.InputTokens,
		"completion_tokens": result.OutputTokens,
		"total_tokens":      result.InputTokens + result.OutputTokens,
	}
}

func (s *Server) streamOpenAIChat(w http.ResponseWriter, model string, result generator.Result) {
	startStream(w)
	id := newID("chatcmpl")
	writeSSE(w, map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"delta":         map[string]string{"role": "assistant"},
			"finish_reason": nil,
		}},
	})
	if result.ToolCall != nil {
		writeSSE(w, map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{{
				"index":         0,
				"delta":         map[string]any{"tool_calls": []map[string]any{openAIToolCall(result.ToolCall)}},
				"finish_reason": nil,
			}},
		})
		writeOpenAIFinish(w, id, model, "tool_calls")
		return
	}
	for _, chunk := range textChunks(result.Text) {
		writeSSE(w, map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{{
				"index":         0,
				"delta":         map[string]string{"content": chunk},
				"finish_reason": nil,
			}},
		})
	}
	writeOpenAIFinish(w, id, model, "stop")
}

func (s *Server) streamOpenAICompletion(w http.ResponseWriter, model string, result generator.Result) {
	startStream(w)
	id := newID("cmpl")
	for _, chunk := range textChunks(result.Text) {
		writeSSE(w, map[string]any{
			"id":      id,
			"object":  "text_completion",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{{
				"text":          chunk,
				"index":         0,
				"finish_reason": nil,
			}},
		})
	}
	writeSSE(w, map[string]any{
		"id":      id,
		"object":  "text_completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"text":          "",
			"index":         0,
			"finish_reason": "stop",
		}},
	})
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}

func writeOpenAIFinish(w http.ResponseWriter, id string, model string, reason string) {
	writeSSE(w, map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"delta":         map[string]string{},
			"finish_reason": reason,
		}},
	})
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}

type claudeRequest struct {
	Model      string          `json:"model"`
	Messages   []claudeMessage `json:"messages"`
	MaxTokens  int             `json:"max_tokens"`
	Stream     bool            `json:"stream"`
	Tools      []claudeTool    `json:"tools"`
	ThinkLevel string          `json:"think_level"`
	Thinking   any             `json:"thinking"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type claudeTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Server) handleClaudeMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req claudeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	conversation := claudeConversationText(req.Messages)
	result := s.generator.Generate(generator.Request{
		Input:        conversation.Input,
		History:      conversation.History,
		MaxTokens:    req.MaxTokens,
		ThinkLevel:   selectThinkLevel(r, req.ThinkLevel, "", req.Thinking),
		Tools:        claudeTools(req.Tools),
		UseInputSeed: true,
		RandomSeed:   time.Now().UnixNano(),
	})
	if req.Stream {
		s.streamClaude(w, req.Model, result)
		return
	}
	content := []map[string]any{{"type": "text", "text": result.Text}}
	stopReason := "end_turn"
	if result.ToolCall != nil {
		stopReason = "tool_use"
		content = []map[string]any{{
			"type":  "tool_use",
			"id":    result.ToolCall.ID,
			"name":  result.ToolCall.Name,
			"input": map[string]string{"command": result.ToolCall.Command},
		}}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":            newID("msg"),
		"type":          "message",
		"role":          "assistant",
		"model":         req.Model,
		"content":       content,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]int{
			"input_tokens":  result.InputTokens,
			"output_tokens": result.OutputTokens,
		},
	})
}

func (s *Server) streamClaude(w http.ResponseWriter, model string, result generator.Result) {
	startStream(w)
	id := newID("msg")
	writeNamedSSE(w, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":      id,
			"type":    "message",
			"role":    "assistant",
			"model":   model,
			"content": []any{},
			"usage": map[string]int{
				"input_tokens":  result.InputTokens,
				"output_tokens": 0,
			},
		},
	})
	if result.ToolCall != nil {
		writeNamedSSE(w, "content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": 0,
			"content_block": map[string]any{
				"type":  "tool_use",
				"id":    result.ToolCall.ID,
				"name":  result.ToolCall.Name,
				"input": map[string]string{"command": result.ToolCall.Command},
			},
		})
		writeNamedSSE(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
		writeNamedSSE(w, "message_delta", map[string]any{
			"type":  "message_delta",
			"delta": map[string]string{"stop_reason": "tool_use"},
			"usage": map[string]int{"output_tokens": result.OutputTokens},
		})
		writeNamedSSE(w, "message_stop", map[string]string{"type": "message_stop"})
		return
	}
	writeNamedSSE(w, "content_block_start", map[string]any{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]string{"type": "text", "text": ""},
	})
	for _, chunk := range textChunks(result.Text) {
		writeNamedSSE(w, "content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]string{"type": "text_delta", "text": chunk},
		})
	}
	writeNamedSSE(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	writeNamedSSE(w, "message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]string{"stop_reason": "end_turn"},
		"usage": map[string]int{"output_tokens": result.OutputTokens},
	})
	writeNamedSSE(w, "message_stop", map[string]string{"type": "message_stop"})
}

type conversationText struct {
	Input   string
	History string
}

func openAIConversationText(messages []openAIMessage) conversationText {
	latestUser := -1
	for i, message := range messages {
		if message.Role == "user" && strings.TrimSpace(textFromAny(message.Content)) != "" {
			latestUser = i
		}
	}
	if latestUser == -1 {
		var input []string
		for _, message := range messages {
			input = append(input, textFromAny(message.Content))
		}
		return conversationText{Input: generator.NormalizePrompt(input...)}
	}
	var history []string
	for _, message := range messages[:latestUser] {
		history = append(history, textFromAny(message.Content))
	}
	return conversationText{
		Input:   generator.NormalizePrompt(textFromAny(messages[latestUser].Content)),
		History: generator.NormalizePrompt(history...),
	}
}

func claudeConversationText(messages []claudeMessage) conversationText {
	latestUser := -1
	for i, message := range messages {
		if message.Role == "user" && strings.TrimSpace(textFromAny(message.Content)) != "" {
			latestUser = i
		}
	}
	if latestUser == -1 {
		var input []string
		for _, message := range messages {
			input = append(input, textFromAny(message.Content))
		}
		return conversationText{Input: generator.NormalizePrompt(input...)}
	}
	var history []string
	for _, message := range messages[:latestUser] {
		history = append(history, textFromAny(message.Content))
	}
	return conversationText{
		Input:   generator.NormalizePrompt(textFromAny(messages[latestUser].Content)),
		History: generator.NormalizePrompt(history...),
	}
}

func textFromAny(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			parts = append(parts, textFromAny(item))
		}
		return generator.NormalizePrompt(parts...)
	case map[string]any:
		if text, ok := v["text"].(string); ok {
			return text
		}
		if content, ok := v["content"]; ok {
			return textFromAny(content)
		}
	}
	return fmt.Sprint(value)
}

func openAITools(tools []openAITool) []generator.Tool {
	var out []generator.Tool
	for _, tool := range tools {
		name := tool.Function.Name
		if name == "" {
			name = tool.Name
		}
		text := strings.ToLower(name + " " + tool.Function.Description + " " + tool.Type)
		out = append(out, generator.Tool{Name: defaultName(name), Bash: isBashTool(text)})
	}
	return out
}

func claudeTools(tools []claudeTool) []generator.Tool {
	var out []generator.Tool
	for _, tool := range tools {
		text := strings.ToLower(tool.Name + " " + tool.Description)
		out = append(out, generator.Tool{Name: defaultName(tool.Name), Bash: isBashTool(text)})
	}
	return out
}

func isBashTool(text string) bool {
	return strings.Contains(text, "bash") || strings.Contains(text, "shell") || strings.Contains(text, "terminal")
}

func defaultName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "bash"
	}
	return name
}

func selectThinkLevel(r *http.Request, explicit string, reasoningEffort string, thinking any) string {
	if header := strings.TrimSpace(r.Header.Get("X-Think-Level")); header != "" {
		return header
	}
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if strings.TrimSpace(reasoningEffort) != "" {
		return reasoningEffort
	}
	switch v := thinking.(type) {
	case string:
		return v
	case map[string]any:
		if enabled, ok := v["enabled"].(bool); ok && !enabled {
			return "none"
		}
		if budget, ok := v["budget_tokens"].(float64); ok && budget > 0 {
			if budget > 4096 {
				return "max"
			}
			return "high"
		}
		if typ, ok := v["type"].(string); ok {
			return typ
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": err.Error()}})
}

func startStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
}

func writeSSE(w http.ResponseWriter, value any) {
	data, _ := json.Marshal(value)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flush(w)
}

func writeNamedSSE(w http.ResponseWriter, event string, value any) {
	data, _ := json.Marshal(value)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	flush(w)
}

func flush(w http.ResponseWriter) {
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func textChunks(text string) []string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return []string{text}
	}
	chunks := make([]string, 0, len(fields))
	for i, field := range fields {
		if i < len(fields)-1 {
			field += " "
		}
		chunks = append(chunks, field)
	}
	return chunks
}

func newID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
