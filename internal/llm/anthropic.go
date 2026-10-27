package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

// Anthropic speaks the Messages API, streamed.
type Anthropic struct {
	ProviderName string
	BaseURL      string // default https://api.anthropic.com
	Key          string
	HTTP         *http.Client
}

func (a *Anthropic) Name() string { return a.ProviderName }

type anthropicContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

func (a *Anthropic) Stream(ctx context.Context, req Request, onDelta func(string) error) (*Response, error) {
	base := strings.TrimRight(a.BaseURL, "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	var msgs []anthropicMessage
	for _, m := range req.Messages {
		switch m.Role {
		case "tool":
			// A tool result is a user turn holding a tool_result block.
			msgs = append(msgs, anthropicMessage{Role: "user", Content: []anthropicContent{{Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Content}}})
		case "assistant":
			var c []anthropicContent
			if m.Content != "" {
				c = append(c, anthropicContent{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				c = append(c, anthropicContent{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: tc.Arguments})
			}
			msgs = append(msgs, anthropicMessage{Role: "assistant", Content: c})
		default:
			msgs = append(msgs, anthropicMessage{Role: "user", Content: []anthropicContent{{Type: "text", Text: m.Content}}})
		}
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	body := map[string]any{
		"model": req.Model, "max_tokens": maxTokens, "messages": msgs, "stream": true, "temperature": req.Temperature,
	}
	if req.System != "" {
		body["system"] = req.System
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = map[string]any{"name": t.Name, "description": t.Description, "input_schema": t.Parameters}
		}
		body["tools"] = tools
	}
	resp, err := post(ctx, a.client(), a.ProviderName, base+"/v1/messages",
		map[string]string{"x-api-key": a.Key, "anthropic-version": "2023-06-01"}, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	out := &Response{Provider: a.ProviderName, Model: req.Model}
	var text strings.Builder
	type block struct {
		kind, id, name string
		args           strings.Builder
	}
	blocks := map[int]*block{}
	err = eachEvent(resp.Body, func(event, data string) error {
		var ev struct {
			Type  string `json:"type"`
			Index int    `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Message struct {
				Model string `json:"model"`
				Usage Usage  `json:"usage"`
			} `json:"message"`
			Usage Usage `json:"usage"`
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return err
		}
		switch ev.Type {
		case "message_start":
			out.Model = ev.Message.Model
			out.Usage.InputTokens = ev.Message.Usage.InputTokens
		case "content_block_start":
			blocks[ev.Index] = &block{kind: ev.ContentBlock.Type, id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
		case "content_block_delta":
			switch ev.Delta.Type {
			case "text_delta":
				text.WriteString(ev.Delta.Text)
				if onDelta != nil {
					return onDelta(ev.Delta.Text)
				}
			case "input_json_delta":
				if b := blocks[ev.Index]; b != nil {
					b.args.WriteString(ev.Delta.PartialJSON)
				}
			}
		case "message_delta":
			out.StopReason = ev.Delta.StopReason
			out.Usage.OutputTokens = ev.Usage.OutputTokens
		case "error":
			status := 500
			if ev.Error.Type == "overloaded_error" {
				status = 529
			}
			return &Error{Provider: a.ProviderName, Status: status, Message: ev.Error.Message}
		}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	idx := make([]int, 0, len(blocks))
	for i := range blocks {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for _, i := range idx {
		if b := blocks[i]; b.kind == "tool_use" {
			args := b.args.String()
			if args == "" {
				args = "{}"
			}
			out.ToolCalls = append(out.ToolCalls, ToolCall{ID: b.id, Name: b.name, Arguments: json.RawMessage(args)})
		}
	}
	out.Text = text.String()
	return out, nil
}

func (a *Anthropic) client() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return http.DefaultClient
}
