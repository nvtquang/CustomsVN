package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Target is a model on a provider.
type Target struct {
	Provider Provider
	Model    string
}

// Router sends each role's calls to its targets in order and fails over to
// the next one when a call fails before its first token. After the first
// token there is no failover: the reader has already seen part of an answer,
// and a second model would start a different one.
//
// A failure cools its provider down for a while by kind: an auth failure for
// the life of the router, a quota failure for a minute, anything else for a
// few seconds.
type Router struct {
	mu       sync.Mutex
	targets  map[string][]Target
	cooldown map[string]time.Time
	Now      func() time.Time
}

// Cooldowns by failure kind.
var cooldowns = map[string]time.Duration{
	FailAuth:    24 * time.Hour,
	FailQuota:   time.Minute,
	FailServer:  10 * time.Second,
	FailNetwork: 10 * time.Second,
}

// NewRouter returns a router with no targets.
func NewRouter() *Router {
	return &Router{targets: map[string][]Target{}, cooldown: map[string]time.Time{}, Now: time.Now}
}

// Set replaces the targets of a role.
func (r *Router) Set(role string, targets ...Target) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.targets[role] = targets
}

// Has reports whether a role has any target.
func (r *Router) Has(role string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.targets[role]) > 0
}

// ErrNoModel is returned for a role with no configured target.
var ErrNoModel = errors.New("no model is configured for this role")

// Stream calls the role's targets in turn.
func (r *Router) Stream(ctx context.Context, role string, req Request, onDelta func(string) error) (*Response, error) {
	r.mu.Lock()
	targets := append([]Target(nil), r.targets[role]...)
	r.mu.Unlock()
	if len(targets) == 0 {
		return nil, fmt.Errorf("%s: %w", role, ErrNoModel)
	}
	var errs []error
	for _, t := range targets {
		name := t.Provider.Name()
		r.mu.Lock()
		until := r.cooldown[name]
		r.mu.Unlock()
		if r.Now().Before(until) {
			errs = append(errs, fmt.Errorf("%s cooling down until %s", name, until.Format(time.TimeOnly)))
			continue
		}
		started := false
		call := req
		call.Model = t.Model
		resp, err := t.Provider.Stream(ctx, call, func(d string) error {
			started = true
			if onDelta == nil {
				return nil
			}
			return onDelta(d)
		})
		if err == nil {
			return resp, nil
		}
		kind := Kind(err)
		if kind == FailCanceled || started || kind == FailRequest {
			return nil, err
		}
		if d, ok := cooldowns[kind]; ok {
			r.mu.Lock()
			r.cooldown[name] = r.Now().Add(d)
			r.mu.Unlock()
		}
		errs = append(errs, err)
	}
	return nil, errors.Join(errs...)
}

// Scripted is a provider for tests: it answers each call with the next reply,
// streamed in pieces, or fails with Err.
type Scripted struct {
	ProviderName string
	Replies      []string
	Err          error
	Calls        []Request
	mu           sync.Mutex
}

func (s *Scripted) Name() string { return s.ProviderName }

func (s *Scripted) Stream(ctx context.Context, req Request, onDelta func(string) error) (*Response, error) {
	s.mu.Lock()
	s.Calls = append(s.Calls, req)
	if s.Err != nil {
		s.mu.Unlock()
		return nil, s.Err
	}
	if len(s.Replies) == 0 {
		s.mu.Unlock()
		return nil, &Error{Provider: s.ProviderName, Status: 500, Message: "no scripted reply left"}
	}
	reply := s.Replies[0]
	s.Replies = s.Replies[1:]
	s.mu.Unlock()
	for i := 0; i < len(reply); {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		j := min(i+16, len(reply))
		for j < len(reply) && reply[j]&0xC0 == 0x80 { // do not split a UTF-8 sequence
			j++
		}
		if onDelta != nil {
			if err := onDelta(reply[i:j]); err != nil {
				return nil, err
			}
		}
		i = j
	}
	return &Response{Provider: s.ProviderName, Model: req.Model, Text: reply, StopReason: "end_turn"}, nil
}
