package trace

import "time"

// Observation kinds: things that can reveal a retrieved memory was used.
const (
	ObservedToolCall      = "tool_call"      // an agent tool call naming memory ids
	ObservedAssistantTurn = "assistant_turn" // an agent reply captured by a hook
)

// Observation is something that happened after a retrieval, normalised so
// detectors need not know which adapter saw it.
type Observation struct {
	Kind    string
	Tool    string   // ObservedToolCall: the tool name
	IDs     []string // ObservedToolCall: memory ids the call named
	Text    string   // ObservedAssistantTurn: the reply text
	Session string
	Project string
	At      time.Time
}
