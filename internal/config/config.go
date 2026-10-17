// Package config persists memtree's settings (a small JSON file) and locates
// the memory database.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/aitakemyjob/memtree/internal/store"
)

// DefaultMCPPort is the loopback port for MCP, the UI and the API.
// (CodeDiver uses 47474.)
const DefaultMCPPort = 47475

// Settings are the user-editable options.
type Settings struct {
	Theme    string `json:"theme"`
	FontSize int    `json:"fontSize"`
	MCPPort  int    `json:"mcpPort"`
	// DBPath overrides the database location ("" = default).
	DBPath string `json:"dbPath"`
	// AgentWrites lets agents save, supersede and archive memories over MCP.
	// Turn it off for a read-only recall setup. Hooks are unaffected.
	AgentWrites bool `json:"agentWrites"`
	// CaptureHooks lets harness hooks record conversation turns.
	CaptureHooks bool `json:"captureHooks"`
	// SurfaceThreshold gates what hooks inject on each prompt (lattice hits
	// scoring below it are dropped). Surfacing stays quiet unless something
	// genuinely relates to the prompt.
	SurfaceThreshold float64 `json:"surfaceThreshold"`
	// MaxSurface caps injected items per prompt.
	MaxSurface int `json:"maxSurface"`
	// LinkHalfLifeDays ages lattice links; faded ones are dropped.
	LinkHalfLifeDays float64 `json:"linkHalfLifeDays"`
	// MaintainHours is how often background maintenance (link decay) runs.
	MaintainHours float64 `json:"maintainHours"`
	// Tiers controls when memories become candidates for compression.
	Tiers store.TierConfig `json:"tiers"`
}

// Default returns the default settings.
func Default() Settings {
	return Settings{
		Theme: "sfc-dark", FontSize: 13, MCPPort: DefaultMCPPort,
		AgentWrites: true, CaptureHooks: true,
		SurfaceThreshold: 0.25, MaxSurface: 3,
		LinkHalfLifeDays: 30, MaintainHours: 12,
		Tiers: store.DefaultTierConfig(),
	}
}

// File is the on-disk document.
type File struct {
	Settings Settings `json:"settings"`
}

// Store is a concurrency-safe settings file.
type Store struct {
	mu   sync.RWMutex
	path string
	file File
}

// DefaultPath is the config file location. MEMTREE_CONFIG overrides it.
func DefaultPath() (string, error) {
	if p := os.Getenv("MEMTREE_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "memtree", "config.json"), nil
}

// DefaultDBPath is where the memory database lives. MEMTREE_DB overrides it.
// It sits in the user data directory, apart from config, so backing up or
// syncing settings never drags the memories along.
func DefaultDBPath() (string, error) {
	if p := os.Getenv("MEMTREE_DB"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "memtree", "memtree.db"), nil
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "memtree", "memtree.db"), nil
		}
		return filepath.Join(home, "AppData", "Local", "memtree", "memtree.db"), nil
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "memtree", "memtree.db"), nil
	}
	return filepath.Join(home, ".local", "share", "memtree", "memtree.db"), nil
}

// Open loads the config at path. A missing file is fine; zero-valued
// settings are back-filled with defaults so older files keep working.
func Open(path string) (*Store, error) {
	s := &Store{path: path, file: File{Settings: Default()}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &s.file); err != nil {
		return nil, err
	}
	s.file.Settings = fill(s.file.Settings)
	return s, nil
}

// fill replaces zero values with defaults. Booleans are not back-filled
// here: Open unmarshals onto the defaults, so an absent field keeps its
// default and only an explicit false turns a switch off.
func fill(s Settings) Settings {
	d := Default()
	if s.Theme == "" {
		s.Theme = d.Theme
	}
	if s.FontSize == 0 {
		s.FontSize = d.FontSize
	}
	if s.MCPPort == 0 {
		s.MCPPort = d.MCPPort
	}
	if s.SurfaceThreshold == 0 {
		s.SurfaceThreshold = d.SurfaceThreshold
	}
	if s.MaxSurface == 0 {
		s.MaxSurface = d.MaxSurface
	}
	if s.LinkHalfLifeDays == 0 {
		s.LinkHalfLifeDays = d.LinkHalfLifeDays
	}
	if s.MaintainHours == 0 {
		s.MaintainHours = d.MaintainHours
	}
	if s.Tiers == (store.TierConfig{}) {
		s.Tiers = d.Tiers
	}
	return s
}

// Get returns a copy of the settings.
func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.file.Settings
}

// Path is the config file path.
func (s *Store) Path() string { return s.path }

// Update applies fn to the settings, validates, and saves atomically.
func (s *Store) Update(fn func(*Settings)) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.file.Settings
	fn(&next)
	next = fill(next)
	if next.MCPPort < 1 || next.MCPPort > 65535 {
		return s.file.Settings, errors.New("port must be between 1 and 65535")
	}
	if next.SurfaceThreshold < 0 || next.SurfaceThreshold > 1 {
		return s.file.Settings, errors.New("surface threshold must be between 0 and 1")
	}
	t := next.Tiers
	if t.AgeHoursToCompact < t.AgeHoursToSemantic || t.ActivationCeilingCompress < 0 || t.ActivationCeilingCompress > 1 || t.MinClusterSize < 1 {
		return s.file.Settings, errors.New("invalid tier settings")
	}
	old := s.file
	s.file.Settings = next
	if err := s.saveLocked(); err != nil {
		s.file = old
		return old.Settings, err
	}
	return next, nil
}

// saveLocked writes the config atomically (temp file + rename); the caller holds the lock.
func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.file, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
