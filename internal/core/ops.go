package core

import (
	"context"
	"errors"

	"github.com/aitakemyjob/memtree/internal/store"
)

type memEvent struct {
	ID string `json:"id"`
	Op string `json:"op"`
}

// AgentWritable returns an error when the user has switched agent writes off.
func (s *Service) AgentWritable() error {
	if !s.Config.Get().AgentWrites {
		return ErrAgentWritesDisabled
	}
	return nil
}

// Save stores a memory. If it summarises a captured session (kind "session"
// with that session's id) the raw capture is retired in its favour.
func (s *Service) Save(ctx context.Context, r store.MemoryRecord) (store.MemoryRecord, error) {
	saved, err := s.Memory.Save(ctx, r)
	if err != nil {
		return saved, err
	}
	if saved.SessionID != "" && saved.Kind == store.KindSession {
		if err := s.Capture.MarkSummarized(ctx, saved.SessionID, saved.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return saved, err
		}
		s.emit(EvCapture, map[string]string{"session": saved.SessionID, "op": "summarized"})
	}
	s.emit(EvMemory, memEvent{saved.ID, "save"})
	s.emit(EvLattice, map[string]string{"op": "link"})
	return saved, nil
}

// Update edits a memory's content; paths non-nil replaces its lattice links.
func (s *Service) Update(ctx context.Context, id string, edit func(*store.MemoryRecord), paths []string) (store.MemoryRecord, error) {
	r, err := s.Memory.Update(ctx, id, edit, paths)
	if err == nil {
		s.emit(EvMemory, memEvent{id, "update"})
		if paths != nil {
			s.emit(EvLattice, map[string]string{"op": "relink"})
		}
	}
	return r, err
}

// SetPinned pins or unpins a memory.
func (s *Service) SetPinned(ctx context.Context, id string, pinned bool) error {
	return s.after(id, "pin", s.Memory.SetPinned(ctx, id, pinned))
}

// Archive hides a memory from recall without deleting it.
func (s *Service) Archive(ctx context.Context, id string) error {
	return s.after(id, "archive", s.Memory.Archive(ctx, id))
}

// Restore undoes Archive.
func (s *Service) Restore(ctx context.Context, id string) error {
	return s.after(id, "restore", s.Memory.Restore(ctx, id))
}

// Supersede marks old as replaced by newer, keeping the original.
func (s *Service) Supersede(ctx context.Context, oldID, newID string) error {
	return s.after(oldID, "supersede", s.Memory.Supersede(ctx, oldID, newID))
}

// Unsupersede undoes Supersede.
func (s *Service) Unsupersede(ctx context.Context, id string) error {
	return s.after(id, "unsupersede", s.Memory.Unsupersede(ctx, id))
}

// Delete permanently removes a memory. A human action; agents archive.
func (s *Service) Delete(ctx context.Context, id string) error {
	err := s.after(id, "delete", s.Memory.Delete(ctx, id))
	if err == nil {
		s.emit(EvLattice, map[string]string{"op": "unlink"})
	}
	return err
}

// Touch reinforces memories the caller actually relied on.
func (s *Service) Touch(ctx context.Context, ids []string) error {
	err := s.Memory.Touch(ctx, ids)
	if err == nil {
		for _, id := range ids {
			s.emit(EvMemory, memEvent{id, "touch"})
		}
	}
	return err
}

// Compress saves an agent-written gist and supersedes its sources.
func (s *Service) Compress(ctx context.Context, ids []string, gist store.MemoryRecord) (store.MemoryRecord, error) {
	saved, err := s.Memory.Compress(ctx, ids, gist)
	if err == nil {
		s.emit(EvMemory, memEvent{saved.ID, "compress"})
		s.emit(EvLattice, map[string]string{"op": "link"})
	}
	return saved, err
}

// TagMemory sets a memory's lattice paths (classification) and clears it
// from the untagged queue.
func (s *Service) TagMemory(ctx context.Context, id string, paths []string, source string) error {
	r, err := s.Memory.Get(ctx, id)
	if err != nil {
		return err
	}
	if len(cleanPaths(paths)) == 0 {
		return errors.New("tag needs at least one category path")
	}
	if err := s.Lattice.SetLinks(ctx, store.ItemMemory, id, paths, r.Summary, source); err != nil {
		return err
	}
	s.emit(EvLattice, map[string]string{"op": "tag"})
	s.emit(EvMemory, memEvent{id, "tag"})
	return nil
}

// MergeCategory folds one lattice node into another.
func (s *Service) MergeCategory(ctx context.Context, from, to string) error {
	err := s.Lattice.MergeNode(ctx, from, to)
	if err == nil {
		s.emit(EvLattice, map[string]string{"op": "merge"})
	}
	return err
}

// PruneCategories removes empty non-seed leaf nodes.
func (s *Service) PruneCategories(ctx context.Context) (int, error) {
	n, err := s.Lattice.PruneEmpty(ctx, store.AxisSeeds)
	if err == nil && n > 0 {
		s.emit(EvLattice, map[string]string{"op": "prune"})
	}
	return n, err
}

// after publishes a memory event when a lifecycle operation succeeded.
func (s *Service) after(id, op string, err error) error {
	if err == nil {
		s.emit(EvMemory, memEvent{id, op})
	}
	return err
}

// cleanPaths normalises category paths, dropping ones that normalise to nothing.
func cleanPaths(in []string) []string {
	var out []string
	for _, p := range in {
		if n := store.NormalizePath(p); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// CaptureTurn records one conversation turn from a harness hook. It is a
// no-op when capture is switched off.
func (s *Service) CaptureTurn(ctx context.Context, harness, session, project, role, text string) error {
	if !s.Config.Get().CaptureHooks {
		return nil
	}
	if err := s.Capture.AddTurn(ctx, harness, session, project, role, text); err != nil {
		return err
	}
	s.emit(EvCapture, map[string]string{"session": session, "op": "turn"})
	return nil
}

// CloseSession ends a captured session and files its raw capture memory.
func (s *Service) CloseSession(ctx context.Context, session, reason string) error {
	if !s.Config.Get().CaptureHooks {
		return nil
	}
	rec, err := s.Capture.Close(ctx, session, reason)
	if err != nil {
		return err
	}
	s.emit(EvCapture, map[string]string{"session": session, "op": "closed"})
	if rec != nil {
		s.emit(EvMemory, memEvent{rec.ID, "capture"})
	}
	return nil
}
