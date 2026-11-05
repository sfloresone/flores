package trajectory

import (
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/sfloresone/flores/apps/cli/internal/agent"
	"github.com/sfloresone/flores/apps/cli/internal/provider"
)

// SchemaVersion is the contract this package writes.
const SchemaVersion = "trajectory/v1"

// Session describes the run for the session.start event.
type Session struct {
	AgentName, AgentVersion string
	Model                   string
	ProviderName            string
	Protocol                string // e.g. "anthropic-messages"
	BaseURL                 string
	Cwd                     string
}

// Emitter writes trajectory events to w. It is not safe for concurrent use.
type Emitter struct {
	w     io.Writer
	start time.Time
	seq   int
	err   error // first write error; later writes are skipped
}

// New starts a trajectory and writes session.start.
func New(w io.Writer, s Session) *Emitter {
	e := &Emitter{w: w, start: time.Now()}
	e.emit("session.start", map[string]any{
		"schema_version": SchemaVersion,
		"agent":          map[string]any{"name": s.AgentName, "version": s.AgentVersion},
		"model":          s.Model,
		"provider":       map[string]any{"name": s.ProviderName, "protocol": s.Protocol, "base_url": s.BaseURL},
		"cwd":            s.Cwd,
		"started_at":     e.start.UTC().Format(time.RFC3339Nano),
	})
	return e
}

// Record writes the trajectory events for one agent event. EventDone writes
// session.end, which must be the last call.
func (e *Emitter) Record(ev agent.Event) {
	switch ev.Kind {
	case agent.EventStepStart:
		e.emit("step.start", map[string]any{"step": ev.Step})
	case agent.EventRequest:
		r := ev.Request
		e.emit("llm.request", map[string]any{
			"step": ev.Step, "request_bytes": r.Bytes, "messages": r.Messages,
			"tools": r.Tools, "system_bytes": r.SystemBytes, "attempt": 1,
		})
	case agent.EventResponse:
		e.response(ev.Step, ev.Response)
	case agent.EventToolCall:
		c := ev.ToolCall
		var args any = string(c.Input) // invalid JSON is kept as a string
		if json.Valid(c.Input) {
			args = c.Input
		}
		e.emit("tool.call", map[string]any{"step": ev.Step, "id": c.ID, "name": c.Name, "arguments": args})
	case agent.EventToolResult:
		r := ev.ToolResult
		fields := map[string]any{
			"step": ev.Step, "id": r.CallID, "ok": !r.IsError,
			"duration_ms": ms(ev.ToolDuration), "output_bytes": len(r.Content),
		}
		if r.IsError {
			fields["error"] = r.Content
		}
		e.emit("tool.result", fields)
	case agent.EventDone:
		e.end(ev.Result)
	}
}

func (e *Emitter) response(step int, r *agent.ResponseStats) {
	fields := map[string]any{"step": step, "duration_ms": ms(r.Duration), "usage": r.Usage}
	if r.TTFT > 0 {
		fields["ttft_ms"] = ms(r.TTFT)
	} else {
		fields["ttft_ms"] = nil
	}
	if r.HTTPStatus != 0 {
		fields["http_status"] = r.HTTPStatus
	}
	if r.Err != nil {
		fields["status"] = "error"
		e.emit("llm.response", fields)
		var pe *provider.Error
		if errors.As(r.Err, &pe) {
			e.emit("error", map[string]any{"kind": "provider", "message": pe.Error(), "retryable": pe.Retryable()})
		}
		return
	}
	fields["status"] = "ok"
	fields["stop_reason"] = r.Stop
	e.emit("llm.response", fields)
	if r.Text != "" {
		e.emit("message", map[string]any{"step": step, "role": "assistant", "text": r.Text})
	}
}

func (e *Emitter) end(res *agent.Result) {
	switch res.Status {
	case agent.StatusMaxSteps:
		e.emit("error", map[string]any{"kind": "limit", "message": "reached --max-steps", "retryable": false})
	case agent.StatusRepetition:
		e.emit("error", map[string]any{"kind": "limit", "message": "repeated tool call", "retryable": false})
	case agent.StatusError:
		if res.Err != nil && !isProvider(res.Err) { // provider errors were reported with their response
			e.emit("error", map[string]any{"kind": "internal", "message": res.Err.Error(), "retryable": false})
		}
	}
	e.emit("session.end", map[string]any{
		"status": string(res.Status), "steps": res.Steps, "duration_ms": ms(time.Since(e.start)),
		"usage": res.Usage, "tool_calls": res.ToolCalls, "tool_errors": res.ToolErrors,
		"final_text": res.FinalText,
	})
}

func isProvider(err error) bool {
	var pe *provider.Error
	return errors.As(err, &pe)
}

// Err returns the first error writing the stream, if any.
func (e *Emitter) Err() error { return e.err }

func (e *Emitter) emit(typ string, fields map[string]any) {
	fields["seq"] = e.seq
	fields["t_ms"] = ms(time.Since(e.start))
	fields["type"] = typ
	e.seq++
	if e.err != nil {
		return
	}
	line, err := json.Marshal(fields)
	if err == nil {
		_, err = e.w.Write(append(line, '\n'))
	}
	e.err = err
}

// ms converts a duration to milliseconds with microsecond precision.
func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
