package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nvtquang/CustomsVN/internal/answer"
	"github.com/nvtquang/CustomsVN/internal/mcpserver"
	"github.com/nvtquang/CustomsVN/internal/tools"
)

// TokenEnv names the bearer token other programs use for /v1 and /mcp. A
// logged-in browser session works too; without either they are closed.
const TokenEnv = "CUSTOMSVN_API_TOKEN"

// mcpHandler serves MCP over streamable HTTP.
func (s *Server) mcpHandler(version string) http.Handler {
	// One server for every session; usage is recorded without a user, as
	// for the token, since MCP calls carry no session of their own.
	server := mcpserver.New(version, s.Tools, s.Answerer, func(ctx context.Context, res *answer.Result) {
		s.RecordUsage(ctx, 0, "mcp:", res)
	})
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
}

// ModelID is the model name /v1 answers as.
const ModelID = "customsvn"

func (s *Server) models(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": []map[string]any{
		{"id": ModelID, "object": "model", "created": 0, "owned_by": "customsvn"}}})
}

type completionRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
	Stream bool `json:"stream"`
	// AsOf is an extension: the day the law is applied as of.
	AsOf string `json:"as_of,omitempty"`
}

// text reads a message content that is a string or a list of text parts.
func text(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return ""
}

// chatCompletions answers like an OpenAI chat model, so tools built for one
// (Open WebUI, Dify, LibreChat) can use CustomsVN as a model. The last user
// message is the question and the earlier turns are its history. A stream
// sends each verified claim as it is verified.
func (s *Server) chatCompletions(w http.ResponseWriter, r *http.Request) {
	var in completionRequest
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		openAIError(w, http.StatusBadRequest, err.Error())
		return
	}
	var history []answer.Turn
	question := ""
	for i, m := range in.Messages {
		t := strings.TrimSpace(text(m.Content))
		if m.Role == "system" || t == "" {
			continue
		}
		if i == len(in.Messages)-1 && m.Role == "user" {
			question = t
			continue
		}
		if m.Role == "user" || m.Role == "assistant" {
			history = append(history, answer.Turn{Role: m.Role, Content: t})
		}
	}
	if question == "" {
		openAIError(w, http.StatusBadRequest, "the last message must be a user message")
		return
	}
	if !s.allowQuestion(w, r) {
		return
	}
	id := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()
	chunk := func(delta map[string]any, finish any) map[string]any {
		return map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": ModelID,
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}}}
	}
	ctx := r.Context()
	if !in.Stream {
		res, err := s.Answerer.Answer(ctx, answer.Input{Question: question, History: history, AsOf: in.AsOf}, func(answer.Event) error { return nil })
		s.RecordUsage(ctx, ownerOf(r), "api:", res)
		if err != nil && res == nil {
			openAIError(w, statusForAnswer(err), err.Error())
			return
		}
		in, out := tokens(res)
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "object": "chat.completion", "created": created, "model": ModelID,
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": mcpserver.Render(res)}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": in, "completion_tokens": out, "total_tokens": in + out}})
		return
	}
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}
	send(chunk(map[string]any{"role": "assistant", "content": ""}, nil))
	first := true
	res, err := s.Answerer.Answer(ctx, answer.Input{Question: question, History: history, AsOf: in.AsOf}, func(e answer.Event) error {
		if e.Type == "claim" {
			piece := strings.TrimSpace(e.Claim.Text)
			for _, n := range claimMarkers(*e.Claim) {
				piece += fmt.Sprintf(" [%d]", n)
			}
			if !first {
				piece = "\n\n" + piece
			}
			first = false
			send(chunk(map[string]any{"content": piece}, nil))
		}
		return ctx.Err()
	})
	s.RecordUsage(ctx, ownerOf(r), "api:", res)
	switch {
	case err != nil && res == nil:
		send(chunk(map[string]any{"content": "Lỗi: " + err.Error()}, nil))
	default:
		// The grounds and anything that was not a claim (a refusal, a
		// question back, cards) come at the end, rendered as text.
		full := mcpserver.Render(res)
		if done := res.Markdown(); len(res.Claims) > 0 && strings.HasPrefix(full, done) {
			full = full[len(done):]
		} else if !first {
			full = "\n\n" + full
		}
		send(chunk(map[string]any{"content": full}, nil))
	}
	send(chunk(map[string]any{}, "stop"))
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

// tokens totals what every model call of an answer used.
func tokens(res *answer.Result) (in, out int) {
	for _, u := range res.Usage {
		in += u.InputTokens
		out += u.OutputTokens
	}
	return in, out
}

func claimMarkers(c answer.Claim) []int {
	var out []int
	seen := map[int]bool{}
	for _, ct := range c.Citations {
		if !seen[ct.N] {
			seen[ct.N] = true
			out = append(out, ct.N)
		}
	}
	return out
}

func statusForAnswer(err error) int {
	if answer.IsNoModel(err) {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadGateway
}

func openAIError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": msg, "type": "invalid_request_error"}})
}

// hs serves the HS lookup screen.
func (s *Server) hs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	res, err := s.Tools.HS(r.Context(), q, 30)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// tool runs one tool for the chat screen's cards and the document viewer.
func (s *Server) tool(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var body struct {
		Args json.RawMessage `json:"args"`
		AsOf string          `json:"as_of"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	res, err := s.Tools.Call(r.Context(), name, body.Args, body.AsOf)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, tools.ErrUnknownTool) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": res.Text, "summary": res.Summary, "card": res.Card})
}
