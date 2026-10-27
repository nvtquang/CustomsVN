package llm

import (
	"context"
	"strings"

	"github.com/tamnd/luatdo/api"
	"github.com/tamnd/luatdo/route"
)

// Luatdo adapts a luatdo route file as one provider, so the endpoints, the
// cooldowns and the cost accounting already set up for luatdo work here too.
// luatdo's completers return whole texts, so the answer arrives as one delta
// and tool calls are not available through it.
type Luatdo struct {
	ProviderName string
	Router       *route.Router
}

// NewLuatdo loads a luatdo routes file (route.DefaultPath when path is empty).
func NewLuatdo(name, path string) (*Luatdo, error) {
	if path == "" {
		path = route.DefaultPath()
	}
	routes, err := route.Load(path)
	if err != nil {
		return nil, err
	}
	return &Luatdo{ProviderName: name, Router: route.New(routes)}, nil
}

func (l *Luatdo) Name() string { return l.ProviderName }

func (l *Luatdo) Stream(ctx context.Context, req Request, onDelta func(string) error) (*Response, error) {
	// The Responses-style request has one input; the turns are written out.
	var input strings.Builder
	for _, m := range req.Messages {
		if len(req.Messages) > 1 {
			input.WriteString(strings.ToUpper(m.Role) + ": ")
		}
		input.WriteString(m.Content)
		input.WriteString("\n\n")
	}
	resp, err := l.Router.Complete(ctx, api.Request{Model: req.Model, Instructions: req.System, Input: strings.TrimSpace(input.String())})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &Error{Provider: l.ProviderName, Message: err.Error()}
	}
	if onDelta != nil {
		if err := onDelta(resp.Text); err != nil {
			return nil, err
		}
	}
	return &Response{Provider: l.ProviderName + "/" + resp.Route, Model: resp.Model, Text: resp.Text, StopReason: "end_turn",
		Usage: Usage{InputTokens: resp.InputTokens, OutputTokens: resp.OutputTokens}}, nil
}
