package core

import (
	"os"
	"path/filepath"
)

// ResolveProject maps a working directory to the project key memories are
// scoped by: the nearest enclosing git repository root, or the directory
// itself outside a repo. Empty input means global scope. Symlinks are
// resolved so the same repo reached two ways is one project.
func ResolveProject(cwd string) string {
	if cwd == "" {
		return ""
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	for dir := abs; ; {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs
		}
		dir = parent
	}
}
