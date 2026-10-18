package trace

import (
	"context"
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)

// fakeStore is an in-memory Reader, UseWriter and Pruner.
type fakeStore struct {
	events []Event
	uses   []Use
	pruned time.Time
	filter Filter
}

func (f *fakeStore) List(_ context.Context, fl Filter) ([]Event, error) {
	f.filter = fl
	return append([]Event{}, f.events...), nil
}
func (f *fakeStore) Get(context.Context, string) (Event, error)        { return Event{}, errors.New("unused") }
func (f *fakeStore) Heat(context.Context, Filter) ([]HeatRow, error)   { return nil, nil }
func (f *fakeStore) Daily(context.Context, Filter) ([]DayCount, error) { return nil, nil }
func (f *fakeStore) MarkUsed(_ context.Context, u []Use) error {
	f.uses = append(f.uses, u...)
	return nil
}
func (f *fakeStore) Prune(_ context.Context, before time.Time) (int, error) {
	f.pruned = before
	return 3, nil
}

func ev(id, session, project string, ids ...string) Event {
	e := Event{ID: id, SessionID: session, Project: project}
	for _, m := range ids {
		e.Hits = append(e.Hits, Hit{MemoryID: m})
	}
	return e
}

func TestToolDetectors(t *testing.T) {
	cands := []Event{ev("e2", "s", "/p", "mem_aaaaaa"), ev("e1", "s", "/p", "mem_aaaaaa", "mem_bbbbbb")}
	got := FetchDetector().Detect(Observation{Kind: ObservedToolCall, Tool: "memory_get", IDs: []string{"mem_aaaaaa", "mem_bbbbbb", "mem_cccccc"}, At: t0}, cands)
	if len(got) != 2 || got[0].EventID != "e2" || got[1].EventID != "e1" || got[0].Evidence != EvidenceFetched || got[0].Detail != "memory_get" {
		t.Errorf("fetch should credit the first candidate that returned each id: %+v", got)
	}
	if got := FetchDetector().Detect(Observation{Kind: ObservedToolCall, Tool: "recall", IDs: []string{"mem_aaaaaa"}}, cands); got != nil {
		t.Errorf("other tools are not fetch evidence: %+v", got)
	}
	if got := ActDetector().Detect(Observation{Kind: ObservedToolCall, Tool: "memory_supersede", IDs: []string{"mem_bbbbbb"}}, cands); len(got) != 1 || got[0].Evidence != EvidenceActed {
		t.Errorf("act: %+v", got)
	}
}

func TestCiteDetector(t *testing.T) {
	cands := []Event{ev("e1", "s", "/p", "mem_5183196b88325358")}
	o := Observation{Kind: ObservedAssistantTurn, Text: "Per mem_5183196b88325358 (and mem_5183196b88325358 again), and mem_ffffffffff which was never retrieved."}
	got := CiteDetector{}.Detect(o, cands)
	if len(got) != 1 || got[0].MemoryID != "mem_5183196b88325358" || got[0].Evidence != EvidenceCited {
		t.Errorf("cite: %+v", got)
	}
	if got := (CiteDetector{}).Detect(Observation{Kind: ObservedToolCall, Text: o.Text}, cands); got != nil {
		t.Error("only assistant turns are citations")
	}
}

func TestTrackerPrefersSessionThenProject(t *testing.T) {
	st := &fakeStore{events: []Event{
		ev("newest-other", "x", "/q", "mem_aaaaaa"),
		ev("project", "y", "/p", "mem_aaaaaa"),
		ev("session", "s", "/p", "mem_aaaaaa"),
	}}
	tr := NewUseTracker(st, st, 30*time.Minute, func() time.Time { return t0 }, DefaultDetectors()...)
	uses, err := tr.Observe(context.Background(), Observation{Kind: ObservedToolCall, Tool: "memory_get", IDs: []string{"mem_aaaaaa"}, Session: "s", Project: "/p"})
	if err != nil || len(uses) != 1 || uses[0].EventID != "session" {
		t.Fatalf("same session wins: %+v %v", uses, err)
	}
	if len(st.uses) != 1 || !st.uses[0].At.Equal(t0) {
		t.Errorf("uses written with the observation time: %+v", st.uses)
	}
	if !st.filter.Since.Equal(t0.Add(-30*time.Minute)) || len(st.filter.Origins) != 2 {
		t.Errorf("candidates are recent agent-facing events: %+v", st.filter)
	}
	uses, _ = tr.Observe(context.Background(), Observation{Kind: ObservedToolCall, Tool: "memory_get", IDs: []string{"mem_aaaaaa"}, Project: "/p"})
	if len(uses) != 1 || uses[0].EventID != "project" {
		t.Errorf("then same project: %+v", uses)
	}
	uses, _ = tr.Observe(context.Background(), Observation{Kind: ObservedToolCall, Tool: "memory_get", IDs: []string{"mem_aaaaaa"}})
	if len(uses) != 1 || uses[0].EventID != "newest-other" {
		t.Errorf("then newest: %+v", uses)
	}
}

func TestTrackerAcceptsNewDetectors(t *testing.T) {
	st := &fakeStore{events: []Event{ev("e", "", "", "mem_aaaaaa")}}
	custom := detectorFunc(func(o Observation, c []Event) []Use {
		return []Use{{EventID: c[0].ID, MemoryID: "mem_aaaaaa", Evidence: "custom"}}
	})
	tr := NewUseTracker(st, st, time.Minute, func() time.Time { return t0 }, custom)
	if uses, _ := tr.Observe(context.Background(), Observation{Kind: "anything"}); len(uses) != 1 || uses[0].Evidence != "custom" {
		t.Errorf("custom detector: %+v", uses)
	}
}

type detectorFunc func(Observation, []Event) []Use

func (f detectorFunc) Detect(o Observation, c []Event) []Use { return f(o, c) }

func TestRecorderDecorators(t *testing.T) {
	var stored, published []string
	base := RecorderFunc(func(_ context.Context, e Event) error { stored = append(stored, e.ID); return nil })
	on := true
	r := Gated(func() bool { return on }, Publishing(func(e Event) { published = append(published, e.ID) }, base))
	r.Record(context.Background(), Event{ID: "a"})
	on = false
	r.Record(context.Background(), Event{ID: "b"})
	if len(stored) != 1 || len(published) != 1 || stored[0] != "a" {
		t.Errorf("gated: stored %v published %v", stored, published)
	}
	failing := Publishing(func(Event) { t.Error("must not publish a failed record") },
		RecorderFunc(func(context.Context, Event) error { return errors.New("disk full") }))
	if err := failing.Record(context.Background(), Event{}); err == nil {
		t.Error("error should propagate")
	}
}

func TestRetention(t *testing.T) {
	st := &fakeStore{}
	days := 90
	r := Retention{Pruner: st, Days: func() int { return days }, Now: func() time.Time { return t0 }}
	if n, _ := r.Run(context.Background()); n != 3 || !st.pruned.Equal(t0.AddDate(0, 0, -90)) {
		t.Errorf("cutoff %v n %d", st.pruned, n)
	}
	days, st.pruned = 0, time.Time{}
	if n, _ := r.Run(context.Background()); n != 0 || !st.pruned.IsZero() {
		t.Error("0 days keeps everything")
	}
}

func TestOriginContext(t *testing.T) {
	if o := OriginFrom(context.Background()); o.Kind != OriginInternal {
		t.Errorf("default origin: %+v", o)
	}
	ctx := WithOrigin(context.Background(), Origin{Kind: OriginUI, Tool: "hooks.handle"})
	ctx = WithOrigin(ctx, Origin{Kind: OriginHook, Tool: "UserPromptSubmit"})
	if o := OriginFrom(ctx); o.Kind != OriginHook || o.Tool != "UserPromptSubmit" {
		t.Errorf("inner origin wins: %+v", o)
	}
}
