package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

// OpenAI speaks /chat/completions, streamed: OpenAI itself, and every
// compatible endpoint (OpenRouter, DeepSeek, Together, a local vLLM or Ollama).
type OpenAI struct {
	ProviderName string
	BaseURL      string // default https://api.openai.com/v1
	Key          string
	HTTP         *http.Client
}

func (o *OpenAI) Name() string { return o.ProviderName }

func (o *OpenAI) Stream(ctx context.Context, req Request, onDelta func(string) error) (*Response, error) {
	base := strings.TrimRight(o.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	msgs := []map[string]any{}
	if req.System != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		msg := map[string]any{"role": m.Role, "content": m.Content}
		if m.Role == "tool" {
			msg["tool_call_id"] = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			calls := make([]map[string]any, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				calls[i] = map[string]any{"id": tc.ID, "type": "function",
					"function": map[string]any{"name": tc.Name, "arguments": string(tc.Arguments)}}
			}
			msg["tool_calls"] = calls
		}
		msgs = append(msgs, msg)
	}
	body := map[string]any{
		"model": req.Model, "messages": msgs, "stream": true, "temperature": req.Temperature,
		"stream_options": map[string]any{"include_usage": true},
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = map[string]any{"type": "function",
				"function": map[string]any{"name": t.Name, "description": t.Description, "parameters": t.Parameters}}
		}
		body["tools"] = tools
	}
	header := map[string]string{}
	if o.Key != "" {
		header["Authorization"] = "Bearer " + o.Key
	}
	client := o.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := post(ctx, client, o.ProviderName, base+"/chat/completions", header, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	out := &Response{Provider: o.ProviderName, Model: req.Model}
	var text strings.Builder
	type call struct {
		id, name string
		args     strings.Builder
	}
	calls := map[int]*call{}
	err = eachEvent(resp.Body, func(_, data string) error {
		var ev struct {
			Model   string `json:"model"`
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return err
		}
		if ev.Error != nil {
			return &Error{Provider: o.ProviderName, Status: 500, Message: ev.Error.Message}
		}
		if ev.Model != "" {
			out.Model = ev.Model
		}
		if ev.Usage != nil {
			out.Usage = Usage{InputTokens: ev.Usage.PromptTokens, OutputTokens: ev.Usage.CompletionTokens}
		}
		for _, ch := range ev.Choices {
			if ch.FinishReason != "" {
				out.StopReason = ch.FinishReason
			}
			for _, tc := range ch.Delta.ToolCalls {
				c := calls[tc.Index]
				if c == nil {
					c = &call{}
					calls[tc.Index] = c
				}
				if tc.ID != "" {
					c.id = tc.ID
				}
				if tc.Function.Name != "" {
					c.name = tc.Function.Name
				}
				c.args.WriteString(tc.Function.Arguments)
			}
			if ch.Delta.Content != "" {
				text.WriteString(ch.Delta.Content)
				if onDelta != nil {
					if err := onDelta(ch.Delta.Content); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	idx := make([]int, 0, len(calls))
	for i := range calls {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for _, i := range idx {
		c := calls[i]
		args := c.args.String()
		if args == "" {
			args = "{}"
		}
		out.ToolCalls = append(out.ToolCalls, ToolCall{ID: c.id, Name: c.name, Arguments: json.RawMessage(args)})
	}
	out.Text = text.String()
	return out, nil
}
