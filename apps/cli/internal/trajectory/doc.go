// Package trajectory writes the trajectory/v1 JSONL stream that the
// benchmark suite reads (benchmarks/docs/trajectory-schema.md and
// benchmarks/schema/trajectory.v1.schema.json).
//
// It turns agent events into trajectory events, one JSON object per line,
// with a consecutive seq and a monotonic t_ms.
//
// What this package must NOT do: decide anything about the run. It only
// records what the agent reports.
package trajectory
