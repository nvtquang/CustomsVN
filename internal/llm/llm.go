// Package llm is the model layer: one interface over the wires CustomsVN
// speaks, with streaming, tool calls and failover between providers.
//
// luatdo's route package does failover over completions that return whole
// texts. A chat answer has to stream and an agent has to call tools, so this
// package carries its own Anthropic and OpenAI-compatible wires and adapts a
// luatdo route file as one more provider (see Luatdo), instead of changing
// luatdo.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Roles a model is chosen for.
const (
	RoleAnswer  = "answer"  // composes the answer
	RoleAgent   = "agent"   // calls tools to gather sources; the answer model when unset
	RoleRewrite = "rewrite" // turns a follow-up into a standalone question
	RoleRouter  = "router"  // classifies the question; a cheap model
)

// Message is one turn. Tool results and tool calls ride along so an agent
// loop can replay its history.
type Message struct {
	Role       string     `json:"role"` // user, assistant, tool
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Tool is a function the model may call.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema of the arguments
}

// ToolCall is the model asking for a tool.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Request is one model call.
type Request struct {
	Model       string
	System      string
	Messages    []Message
	Tools       []Tool
	MaxTokens   int
	Temperature float64
}

// Usage is the token accounting of one call.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	// Provider and Model are the target that answered, set by the router so
	// usage can be priced per call when a role fails over.
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// Response is a finished call.
type Response struct {
	Provider   string     `json:"provider"`
	Model      string     `json:"model"`
	Text       string     `json:"text"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	StopReason string     `json:"stop_reason"`
	Usage      Usage      `json:"usage"`
}

// Provider is one endpoint.
type Provider interface {
	Name() string
	// Stream calls the model and hands every text delta to onDelta as it
	// arrives. onDelta may be nil. The returned response holds the whole text.
	Stream(ctx context.Context, req Request, onDelta func(string) error) (*Response, error)
}

// Error is a failed call with what failover needs to know about it.
type Error struct {
	Provider string
	Status   int
	Message  string
}

func (e *Error) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("%s: HTTP %d: %s", e.Provider, e.Status, e.Message)
	}
	return e.Provider + ": " + e.Message
}

// Kinds of failure, so the UI can say which one it was.
const (
	FailAuth     = "auth"     // the key is wrong: retrying will not help
	FailQuota    = "quota"    // rate limited or out of credit
	FailServer   = "server"   // the provider failed
	FailNetwork  = "network"  // could not reach it
	FailRequest  = "request"  // the request was refused as malformed
	FailCanceled = "canceled" // the caller stopped
)

// Kind classifies an error.
func Kind(err error) string {
	if errors.Is(err, context.Canceled) {
		return FailCanceled
	}
	var e *Error
	if !errors.As(err, &e) {
		return FailNetwork
	}
	switch {
	case e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden:
		return FailAuth
	case e.Status == http.StatusTooManyRequests || e.Status == http.StatusPaymentRequired:
		return FailQuota
	case e.Status >= 500 || e.Status == 529:
		return FailServer
	case e.Status >= 400:
		return FailRequest
	}
	return FailNetwork
}
