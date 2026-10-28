package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nvtquang/CustomsVN/internal/answer"
	"github.com/nvtquang/CustomsVN/internal/chat"
	"github.com/nvtquang/CustomsVN/internal/llm"
)

type chatRequest struct {
	ConversationID int64  `json:"conversation_id"`
	Question       string `json:"question"`
	AsOf           string `json:"as_of"`
	// Regenerate replaces the last answer of the conversation.
	Regenerate bool `json:"regenerate"`
}

// chatStream answers over server-sent events:
//
//	meta     {conversation_id, as_of}
//	status   {stage}             rewrite, retrieve, agent, compose
//	intent   {intent}            where the question was routed
//	step     {step}              one agent tool call
//	card     {card}              a status, timeline or HS result, from the data
//	clarify  {text}              a question back; the answer waits for it
//	rewrite  {text}              the standalone question
//	claim    {claim}             a claim whose every quote was verified
//	dropped  {dropped}           a claim the verifier refused
//	refusal  {refusal}
//	done     {message_id, ...}
//	error    {kind, message}
//
// The answer streams claim by claim rather than token by token: a claim is
// sent only once its quotes are found word for word in the chunks it cites.
func (s *Server) chatStream(w http.ResponseWriter, r *http.Request) {
	var in chatRequest
	if !readJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	owner := ownerOf(r)
	in.Question = strings.TrimSpace(in.Question)
	if in.AsOf != "" {
		if _, err := time.Parse(time.DateOnly, in.AsOf); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("as_of must be YYYY-MM-DD"))
			return
		}
	}
	if in.ConversationID != 0 {
		if err := s.Chat.Owns(ctx, in.ConversationID, owner); err != nil {
			writeError(w, statusOf(err), err)
			return
		}
	}
	switch {
	case in.Regenerate:
		if in.ConversationID == 0 {
			writeError(w, http.StatusBadRequest, errors.New("regenerate needs a conversation_id"))
			return
		}
		q, asOf, err := s.Chat.PopAnswer(ctx, in.ConversationID)
		if err != nil {
			writeError(w, statusOf(err), err)
			return
		}
		in.Question = q
		if in.AsOf == "" {
			in.AsOf = asOf
		}
	case in.Question == "":
		writeError(w, http.StatusBadRequest, errors.New("empty question"))
		return
	case len([]rune(in.Question)) > 2000:
		writeError(w, http.StatusBadRequest, errors.New("the question is longer than 2,000 characters"))
		return
	}
	if !s.allowQuestion(w, r) {
		return
	}

	var history []answer.Turn
	if in.ConversationID == 0 {
		id, err := s.Chat.Create(ctx, owner, in.Question, in.AsOf)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		in.ConversationID = id
	} else {
		h, err := s.Chat.History(ctx, in.ConversationID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if len(h) == 0 && !in.Regenerate {
			if _, _, err := s.Chat.Get(ctx, in.ConversationID); err != nil {
				writeError(w, statusOf(err), err)
				return
			}
		}
		history = h
		if in.AsOf == "" {
			if c, _, err := s.Chat.Get(ctx, in.ConversationID); err == nil {
				in.AsOf = c.AsOf
			}
		}
		if in.Regenerate && len(history) > 0 {
			history = history[:len(history)-1] // the question being answered again
		}
	}
	if !in.Regenerate {
		if _, err := s.Chat.AddUser(ctx, in.ConversationID, in.Question, in.AsOf); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	send := func(event string, v any) error {
		data, _ := json.Marshal(v)
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return ctx.Err()
	}
	asOf := in.AsOf
	if asOf == "" {
		asOf = s.Engine.Now().Format(time.DateOnly)
	}
	_ = send("meta", map[string]any{"conversation_id": in.ConversationID, "as_of": asOf, "question": in.Question})

	res, err := s.Answerer.Answer(ctx, answer.Input{Question: in.Question, History: history, AsOf: asOf}, func(e answer.Event) error {
		return send(e.Type, e)
	})
	// The answer is stored even when the reader stopped it: the claims already
	// shown were verified and the history should say what was said.
	store := context.WithoutCancel(ctx)
	failure := ""
	if err != nil {
		failure = err.Error()
	}
	if res != nil && (len(res.Claims) > 0 || res.Refusal != nil || res.Clarify != "" || len(res.Cards) > 0) {
		id, serr := s.Chat.AddAnswer(store, in.ConversationID, res, failure)
		if serr != nil {
			s.Log.Error("store answer", "err", serr)
		}
		if err == nil {
			_ = send("done", map[string]any{"message_id": id, "conversation_id": in.ConversationID, "refused": res.Refusal != nil,
				"dropped": len(res.Dropped), "first_claim_ms": res.FirstClaimMS, "timing_ms": res.TimingMS,
				"corpus_version": res.CorpusVersion, "model": res.Model, "clarify": res.Clarify != ""})
			return
		}
	}
	if err != nil {
		kind := llm.Kind(err)
		if answer.IsNoModel(err) {
			kind = "no_model"
		}
		if kind != llm.FailCanceled {
			s.Log.Warn("answer failed", "kind", kind, "err", err)
		}
		_ = send("error", map[string]string{"kind": kind, "message": err.Error()})
	}
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	list, err := s.Chat.List(r.Context(), ownerOf(r), r.URL.Query().Get("q"), 200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Chat.Owns(r.Context(), id, ownerOf(r)); err != nil {
		writeError(w, statusOf(err), err)
		return
	}
	c, msgs, err := s.Chat.Get(r.Context(), id)
	if err != nil {
		writeError(w, statusOf(err), err)
		return
	}
	if msgs == nil {
		msgs = []chat.Message{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversation": c, "messages": msgs})
}

func (s *Server) renameConversation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Title *string `json:"title"`
		AsOf  *string `json:"as_of_date"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.Chat.Owns(r.Context(), id, ownerOf(r)); err != nil {
		writeError(w, statusOf(err), err)
		return
	}
	if in.Title != nil {
		if err := s.Chat.Rename(r.Context(), id, *in.Title); err != nil {
			writeError(w, statusOf(err), err)
			return
		}
	}
	if in.AsOf != nil {
		if *in.AsOf != "" {
			if _, err := time.Parse(time.DateOnly, *in.AsOf); err != nil {
				writeError(w, http.StatusBadRequest, errors.New("as_of_date must be YYYY-MM-DD"))
				return
			}
		}
		if err := s.Chat.SetAsOf(r.Context(), id, *in.AsOf); err != nil {
			writeError(w, statusOf(err), err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Chat.Owns(r.Context(), id, ownerOf(r)); err != nil {
		writeError(w, statusOf(err), err)
		return
	}
	if err := s.Chat.Delete(r.Context(), id); err != nil {
		writeError(w, statusOf(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) feedback(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MessageID int64  `json:"message_id"`
		Rating    int    `json:"rating"`
		Reason    string `json:"reason"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.Chat.Feedback(r.Context(), ownerOf(r), in.MessageID, in.Rating, in.Reason); err != nil {
		writeError(w, statusOf(err), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("bad id"))
		return 0, false
	}
	return id, true
}

func statusOf(err error) int {
	if errors.Is(err, chat.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
