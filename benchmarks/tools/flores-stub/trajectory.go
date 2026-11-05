package main

import (
	"encoding/json"
	"io"
	"time"
)

// emitter writes trajectory/v0 events as JSON lines. When disabled (text
// output mode) it still tracks time so session.end durations stay correct.
type emitter struct {
	w       io.Writer
	enabled bool
	start   time.Time
	seq     int
}

func newEmitter(w io.Writer, enabled bool) *emitter {
	return &emitter{w: w, enabled: enabled, start: time.Now()}
}

func (e *emitter) elapsed() float64 { return msSince(e.start) }

func (e *emitter) emit(typ string, fields map[string]any) {
	ev := make(map[string]any, len(fields)+3)
	for k, v := range fields {
		ev[k] = v
	}
	ev["seq"] = e.seq
	ev["t_ms"] = e.elapsed()
	ev["type"] = typ
	e.seq++
	if !e.enabled {
		return
	}
	line, _ := json.Marshal(ev)
	_, _ = e.w.Write(append(line, '\n'))
}
