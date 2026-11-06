package agent

import (
	"hash/fnv"

	"github.com/sfloresone/flores/apps/cli/internal/provider"
)

// Repetition detection (ADR 0003): if the same tool call gets the same result
// repeatLimit times within the last repeatWindow steps, the model is looping.
// The window does not need to be full, so an exact loop stops at step 5.
const (
	repeatWindow = 10
	repeatLimit  = 5
)

// guard decides, after each step with tool calls, whether a brake fires.
type guard struct {
	maxSteps int        // 0 = no limit
	steps    int        // steps seen in this turn
	window   [][]uint64 // signatures of the last repeatWindow steps
}

func newGuard(maxSteps int) *guard {
	return &guard{maxSteps: maxSteps}
}

// afterStep records a step and returns the brake that fires, or "".
func (g *guard) afterStep(calls []provider.ToolCall, results []provider.ToolResult) Status {
	g.steps++
	sigs := make([]uint64, len(calls))
	for i := range calls {
		sigs[i] = signature(calls[i], results[i])
	}
	g.window = append(g.window, sigs)
	if len(g.window) > repeatWindow {
		g.window = g.window[1:]
	}

	if g.maxSteps > 0 && g.steps >= g.maxSteps {
		return StatusMaxSteps
	}
	counts := map[uint64]int{}
	for _, step := range g.window {
		for _, s := range step {
			counts[s]++
			if counts[s] >= repeatLimit {
				return StatusRepetition
			}
		}
	}
	return ""
}

// signature hashes what makes two tool calls "the same": name, arguments and
// result. Hashing keeps memory constant however large the outputs are.
func signature(call provider.ToolCall, res provider.ToolResult) uint64 {
	h := fnv.New64a()
	h.Write([]byte(call.Name))
	h.Write([]byte{0})
	h.Write(call.Input)
	h.Write([]byte{0})
	h.Write([]byte(res.Content))
	if res.IsError {
		h.Write([]byte{1})
	}
	return h.Sum64()
}
