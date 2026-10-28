// Package jobs runs the long data tasks (prep, ingest, embed) in the
// background of serve, one at a time, with progress and a log the admin
// screen can follow. Jobs live in the ingest_jobs table, so the queue and the
// logs survive a restart; a job that was running when the server stopped is
// marked failed on the next start.
package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Statuses.
const (
	Queued   = "queued"
	Running  = "running"
	Done     = "done"
	Failed   = "failed"
	Canceled = "canceled"
)

// maxLog bounds a job's stored log; the head and the tail are kept.
const maxLog = 512 << 10

// Func does one kind of job. Every line written to log is kept, and the last
// one is the job's progress. A Func should stop when ctx is canceled.
type Func func(ctx context.Context, params json.RawMessage, log io.Writer) error

// Job is a stored job.
type Job struct {
	ID         int64           `json:"id"`
	Kind       string          `json:"kind"`
	Params     json.RawMessage `json:"params"`
	Status     string          `json:"status"`
	Progress   string          `json:"progress"`
	Log        string          `json:"log,omitempty"`
	User       string          `json:"user,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
}

// Runner takes queued jobs one by one.
type Runner struct {
	Pool  *pgxpool.Pool
	Log   *slog.Logger
	Kinds map[string]Func
	// After runs when a job of a kind finishes well, e.g. to reload the
	// corpus after an ingest.
	After func(ctx context.Context, kind string) error

	wake    chan struct{}
	mu      sync.Mutex
	running int64
	cancel  context.CancelFunc
}

// Start marks jobs left running by a previous process as failed and starts
// taking jobs until ctx ends.
func (r *Runner) Start(ctx context.Context) error {
	r.wake = make(chan struct{}, 1)
	if _, err := r.Pool.Exec(ctx, `UPDATE ingest_jobs SET status = 'failed', finished_at = now(),
		log = log || E'\n--- interrupted: the server stopped while this job ran' WHERE status = 'running'`); err != nil {
		return err
	}
	go r.loop(ctx)
	return nil
}

func (r *Runner) loop(ctx context.Context) {
	for {
		for {
			ok, err := r.next(ctx)
			if err != nil && ctx.Err() == nil {
				r.Log.Error("job", "err", err)
			}
			if !ok || ctx.Err() != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		case <-time.After(time.Minute):
		}
	}
}

// Enqueue adds a job. A kind the runner does not know is refused.
func (r *Runner) Enqueue(ctx context.Context, kind string, params json.RawMessage, user int64) (int64, error) {
	if _, ok := r.Kinds[kind]; !ok {
		return 0, fmt.Errorf("unknown job kind %q", kind)
	}
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	var id int64
	err := r.Pool.QueryRow(ctx, `INSERT INTO ingest_jobs (kind, params, status, user_id) VALUES ($1, $2, 'queued', nullif($3::bigint, 0)) RETURNING id`,
		kind, params, user).Scan(&id)
	if err != nil {
		return 0, err
	}
	select {
	case r.wake <- struct{}{}:
	default:
	}
	return id, nil
}

// Cancel stops a running job or drops a queued one.
func (r *Runner) Cancel(ctx context.Context, id int64) error {
	tag, err := r.Pool.Exec(ctx, `UPDATE ingest_jobs SET status = 'canceled', finished_at = now() WHERE id = $1 AND status = 'queued'`, id)
	if err != nil || tag.RowsAffected() > 0 {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running == id && r.cancel != nil {
		r.cancel()
		return nil
	}
	return errors.New("the job is neither queued nor running")
}

// next runs the oldest queued job, if any.
func (r *Runner) next(ctx context.Context) (bool, error) {
	var j Job
	err := r.Pool.QueryRow(ctx, `UPDATE ingest_jobs SET status = 'running', started_at = now()
		WHERE id = (SELECT id FROM ingest_jobs WHERE status = 'queued' ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING id, kind, params`).Scan(&j.ID, &j.Kind, &j.Params)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	jctx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.running, r.cancel = j.ID, cancel
	r.mu.Unlock()
	defer func() {
		cancel()
		r.mu.Lock()
		r.running, r.cancel = 0, nil
		r.mu.Unlock()
	}()

	w := &logWriter{}
	stop := make(chan struct{})
	flushed := make(chan struct{})
	go func() {
		defer close(flushed)
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if text, progress, changed := w.snapshot(); changed {
					_, _ = r.Pool.Exec(ctx, `UPDATE ingest_jobs SET log = $2, progress = $3 WHERE id = $1`, j.ID, text, progress)
				}
			}
		}
	}()

	r.Log.Info("job started", "id", j.ID, "kind", j.Kind)
	start := time.Now()
	err = r.run(jctx, j, w)
	if err == nil && r.After != nil {
		if aerr := r.After(jctx, j.Kind); aerr != nil {
			err = fmt.Errorf("after %s: %w", j.Kind, aerr)
		}
	}
	close(stop)
	<-flushed
	status := Done
	switch {
	case jctx.Err() != nil && ctx.Err() == nil:
		status = Canceled
		fmt.Fprintf(w, "--- canceled after %s\n", time.Since(start).Round(time.Second))
	case err != nil:
		status = Failed
		fmt.Fprintf(w, "--- failed after %s: %v\n", time.Since(start).Round(time.Second), err)
	default:
		fmt.Fprintf(w, "--- done in %s\n", time.Since(start).Round(time.Second))
	}
	text, progress, _ := w.snapshot()
	r.Log.Info("job finished", "id", j.ID, "kind", j.Kind, "status", status)
	// The server may be stopping; the last write gets its own deadline.
	fctx, fcancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer fcancel()
	_, uerr := r.Pool.Exec(fctx, `UPDATE ingest_jobs SET status = $2, log = $3, progress = $4, finished_at = now() WHERE id = $1`,
		j.ID, status, text, progress)
	return true, uerr
}

// run calls the job's function and turns a panic into a failure, so a bug in
// one job does not take the server down.
func (r *Runner) run(ctx context.Context, j Job, w io.Writer) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return r.Kinds[j.Kind](ctx, j.Params, w)
}

// logWriter keeps what a job writes, bounded, and its last line.
type logWriter struct {
	mu      sync.Mutex
	head    bytes.Buffer
	tail    []byte
	cut     bool
	last    string
	partial []byte
	dirty   bool
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.dirty = true
	if w.head.Len() < maxLog/2 {
		w.head.Write(p)
	} else {
		w.cut = true
		w.tail = append(w.tail, p...)
		if len(w.tail) > maxLog/2 {
			w.tail = append([]byte(nil), w.tail[len(w.tail)-maxLog/2:]...)
		}
	}
	w.partial = append(w.partial, p...)
	if i := bytes.LastIndexByte(w.partial, '\n'); i >= 0 {
		lines := strings.Split(strings.TrimRight(string(w.partial[:i]), "\r\n"), "\n")
		for k := len(lines) - 1; k >= 0; k-- {
			if l := strings.TrimSpace(lines[k]); l != "" {
				w.last = l
				break
			}
		}
		w.partial = append([]byte(nil), w.partial[i+1:]...)
	}
	return len(p), nil
}

func (w *logWriter) snapshot() (text, progress string, changed bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	changed, w.dirty = w.dirty, false
	text = w.head.String()
	if w.cut {
		text += "\n[…]\n" + string(w.tail)
	}
	progress = w.last
	if l := strings.TrimSpace(string(w.partial)); l != "" {
		progress = l
	}
	if r := []rune(progress); len(r) > 300 {
		progress = string(r[:300])
	}
	return strings.ToValidUTF8(text, "�"), strings.ToValidUTF8(progress, "�"), changed
}

const jobCols = `j.id, j.kind, j.params, j.status, j.progress, coalesce(u.email, ''), j.created_at, j.started_at, j.finished_at`

func scan(row pgx.Row, withLog bool) (*Job, error) {
	var j Job
	dest := []any{&j.ID, &j.Kind, &j.Params, &j.Status, &j.Progress, &j.User, &j.CreatedAt, &j.StartedAt, &j.FinishedAt}
	if withLog {
		dest = append(dest, &j.Log)
	}
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return &j, nil
}

// List returns jobs without their logs, newest first.
func List(ctx context.Context, pool *pgxpool.Pool, limit int) ([]Job, error) {
	rows, err := pool.Query(ctx, `SELECT `+jobCols+` FROM ingest_jobs j LEFT JOIN users u ON u.id = j.user_id ORDER BY j.id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		j, err := scan(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// Get returns a job with its log.
func Get(ctx context.Context, pool *pgxpool.Pool, id int64) (*Job, error) {
	return scan(pool.QueryRow(ctx, `SELECT `+jobCols+`, j.log FROM ingest_jobs j LEFT JOIN users u ON u.id = j.user_id WHERE j.id = $1`, id), true)
}
