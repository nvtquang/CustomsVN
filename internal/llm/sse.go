package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// post sends a JSON body and returns the response, turning a non-2xx status
// into an *Error with the provider's message.
func post(ctx context.Context, client *http.Client, provider, url string, header map[string]string, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &Error{Provider: provider, Message: err.Error()}
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &Error{Provider: provider, Status: resp.StatusCode, Message: errorMessage(data)}
	}
	return resp, nil
}

// errorMessage pulls the human part out of a provider's error body.
func errorMessage(data []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &e) == nil {
		if e.Error.Message != "" {
			return e.Error.Message
		}
		if e.Message != "" {
			return e.Message
		}
	}
	s := strings.TrimSpace(string(data))
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// eachEvent reads a server-sent event stream and calls visit with each
// event's name and data. A data line of [DONE] ends the stream.
func eachEvent(r io.Reader, visit func(event, data string) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	event, data := "", strings.Builder{}
	flush := func() error {
		if data.Len() == 0 {
			event = ""
			return nil
		}
		d := data.String()
		data.Reset()
		e := event
		event = ""
		if d == "[DONE]" {
			return io.EOF
		}
		return visit(e, d)
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if err := flush(); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(line[len("data:"):], " "))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if err := flush(); err != nil && err != io.EOF {
		return err
	}
	return nil
}
