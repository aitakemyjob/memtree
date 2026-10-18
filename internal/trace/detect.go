package trace

import "regexp"

// Detector applies one evidence rule: given an observation and the recent
// retrievals it may relate to (newest and same-session first), it returns
// the uses it can vouch for. Adding a rule means adding a Detector.
type Detector interface {
	Detect(o Observation, candidates []Event) []Use
}

// ToolDetector treats a tool call naming a retrieved memory as evidence.
type ToolDetector struct {
	Evidence string
	Tools    map[string]bool
}

// FetchDetector marks memories the agent read in full.
func FetchDetector() ToolDetector {
	return ToolDetector{Evidence: EvidenceFetched, Tools: map[string]bool{"memory_get": true}}
}

// ActDetector marks memories the agent changed.
func ActDetector() ToolDetector {
	return ToolDetector{Evidence: EvidenceActed, Tools: map[string]bool{
		"memory_supersede": true, "memory_update": true, "memory_pin": true, "memory_compress": true,
	}}
}

// Detect implements Detector.
func (d ToolDetector) Detect(o Observation, candidates []Event) []Use {
	if o.Kind != ObservedToolCall || !d.Tools[o.Tool] {
		return nil
	}
	return attribute(o.IDs, candidates, d.Evidence, o.Tool, o)
}

// memoryID matches a memory id in free text.
var memoryID = regexp.MustCompile(`\bmem_[0-9a-f]{6,}\b`)

// CiteDetector marks memories whose id the agent's reply mentions.
type CiteDetector struct{}

// Detect implements Detector.
func (CiteDetector) Detect(o Observation, candidates []Event) []Use {
	if o.Kind != ObservedAssistantTurn {
		return nil
	}
	return attribute(memoryID.FindAllString(o.Text, -1), candidates, EvidenceCited, "", o)
}

// DefaultDetectors are the evidence rules memtree ships with.
func DefaultDetectors() []Detector {
	return []Detector{FetchDetector(), ActDetector(), CiteDetector{}}
}

// attribute credits each id to the first candidate event that returned it.
func attribute(ids []string, candidates []Event, evidence, detail string, o Observation) []Use {
	var out []Use
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		for _, e := range candidates {
			if e.returned(id) {
				out = append(out, Use{EventID: e.ID, MemoryID: id, Evidence: evidence, Detail: detail, At: o.At})
				break
			}
		}
	}
	return out
}

// returned reports whether the event's hits include memory id.
func (e Event) returned(id string) bool {
	for _, h := range e.Hits {
		if h.MemoryID == id {
			return true
		}
	}
	return false
}
