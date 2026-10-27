package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func sse(w http.ResponseWriter, lines ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, l := range lines {
		_, _ = io.WriteString(w, l+"\n\n")
	}
}

func TestAnthropicStreamsTextAndToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "k" {
			t.Errorf("request %s %q", r.URL.Path, r.Header.Get("x-api-key"))
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["system"] != "sys" || body["stream"] != true {
			t.Errorf("body %v", body)
		}
		sse(w,
			`event: message_start`+"\n"+`data: {"type":"message_start","message":{"model":"claude-x","usage":{"input_tokens":12}}}`,
			`event: content_block_start`+"\n"+`data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`event: content_block_delta`+"\n"+`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Xin "}}`,
			`event: content_block_delta`+"\n"+`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"chào"}}`,
			`event: content_block_start`+"\n"+`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"t1","name":"search_legal"}}`,
			`event: content_block_delta`+"\n"+`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"query\":"}}`,
			`event: content_block_delta`+"\n"+`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"hải quan\"}"}}`,
			`event: message_delta`+"\n"+`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}`,
			`event: message_stop`+"\n"+`data: {"type":"message_stop"}`,
		)
	}))
	defer srv.Close()
	a := &Anthropic{ProviderName: "a", BaseURL: srv.URL, Key: "k"}
	var deltas []string
	resp, err := a.Stream(context.Background(), Request{Model: "m", System: "sys", Messages: []Message{{Role: "user", Content: "hi"}}},
		func(d string) error { deltas = append(deltas, d); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Xin chào" || len(deltas) != 2 || resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 7 {
		t.Errorf("resp %+v deltas %v", resp, deltas)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "search_legal" || string(resp.ToolCalls[0].Arguments) != `{"query":"hải quan"}` {
		t.Errorf("tool calls %+v", resp.ToolCalls)
	}
}

func TestOpenAIStreams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("request %s", r.URL.Path)
		}
		sse(w,
			`data: {"model":"gpt-x","choices":[{"delta":{"content":"Điều "}}]}`,
			`data: {"choices":[{"delta":{"content":"18"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"get_provision","arguments":"{\"id\""}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"x\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":3}}`,
			`data: [DONE]`,
		)
	}))
	defer srv.Close()
	o := &OpenAI{ProviderName: "o", BaseURL: srv.URL + "/v1", Key: "k"}
	resp, err := o.Stream(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "q"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Điều 18" || resp.Model != "gpt-x" || resp.Usage.OutputTokens != 3 || resp.StopReason != "tool_calls" {
		t.Errorf("resp %+v", resp)
	}
	if len(resp.ToolCalls) != 1 || string(resp.ToolCalls[0].Arguments) != `{"id":"x"}` {
		t.Errorf("tool calls %+v", resp.ToolCalls)
	}
}

func TestErrorsAreClassified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"slow down"}}`)
	}))
	defer srv.Close()
	_, err := (&OpenAI{ProviderName: "o", BaseURL: srv.URL}).Stream(context.Background(), Request{Model: "m"}, nil)
	if Kind(err) != FailQuota || !strings.Contains(err.Error(), "slow down") {
		t.Errorf("err %v kind %s", err, Kind(err))
	}
}

func TestRouterFailsOverBeforeTheFirstToken(t *testing.T) {
	bad := &Scripted{ProviderName: "bad", Err: &Error{Provider: "bad", Status: 429, Message: "quota"}}
	good := &Scripted{ProviderName: "good", Replies: []string{"ok", "again"}}
	r := NewRouter()
	r.Set(RoleAnswer, Target{bad, "m1"}, Target{good, "m2"})
	resp, err := r.Stream(context.Background(), RoleAnswer, Request{}, nil)
	if err != nil || resp.Text != "ok" || good.Calls[0].Model != "m2" {
		t.Fatalf("%+v %v", resp, err)
	}
	// The quota failure cooled "bad" down: the next call does not try it.
	if _, err := r.Stream(context.Background(), RoleAnswer, Request{}, nil); err != nil || len(bad.Calls) != 1 {
		t.Errorf("bad called %d times, err %v", len(bad.Calls), err)
	}
	r.Now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	_, _ = r.Stream(context.Background(), RoleAnswer, Request{}, nil)
	if len(bad.Calls) != 2 {
		t.Errorf("bad not retried after its cooldown")
	}
}

func TestRouterWithoutModel(t *testing.T) {
	if _, err := NewRouter().Stream(context.Background(), RoleRewrite, Request{}, nil); !errors.Is(err, ErrNoModel) {
		t.Errorf("got %v", err)
	}
}
