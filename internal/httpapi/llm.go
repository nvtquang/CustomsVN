package httpapi

import (
	"errors"
	"net/http"

	"github.com/nvtquang/CustomsVN/internal/llmconfig"
	"github.com/nvtquang/CustomsVN/internal/secret"
)

func (s *Server) getLLMSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.LLM.Load(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// putLLMSettings saves providers and roles, then rebuilds the router so the
// next question uses them. The response carries the settings with keys
// masked, never the key that was sent.
func (s *Server) putLLMSettings(w http.ResponseWriter, r *http.Request) {
	var in llmconfig.Settings
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.LLM.Save(r.Context(), &in); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, secret.ErrNoMasterKey) {
			status = http.StatusConflict
		}
		writeError(w, status, err)
		return
	}
	problems, err := s.ReloadLLM(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	st, err := s.LLM.Load(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	msgs := []string{}
	for _, p := range problems {
		msgs = append(msgs, p.Error())
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": st, "problems": msgs})
}

func (s *Server) testLLM(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	writeJSON(w, http.StatusOK, s.LLM.Test(r.Context(), in.Provider, in.Model))
}
