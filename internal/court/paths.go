// Package court provides Court runtime functionality.
package court

import (
	"os"
	"path/filepath"
	"strings"
)

const projectConfigDirName = ".court"

// DefaultConfigDir provides Court runtime functionality.
func DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", wrapErr("resolve home directory", err)
	}
	return filepath.Join(home, ".config", "court"), nil
}

// DefaultDataDir provides Court runtime functionality.
func DefaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", wrapErr("resolve home directory", err)
	}
	return filepath.Join(home, ".local", "share", "court"), nil
}

// DiscoverProjectConfigDirs provides Court runtime functionality.
func DiscoverProjectConfigDirs(workspace string) ([]string, error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = "."
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, wrapErr("resolve workspace", err)
	}
	info, err := os.Stat(abs)
	if err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}

	var chain []string
	for {
		chain = append(chain, filepath.Join(abs, projectConfigDirName))
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}

	var out []string
	for i := len(chain) - 1; i >= 0; i-- {
		if info, err := os.Stat(chain[i]); err == nil && info.IsDir() {
			out = append(out, chain[i])
		}
	}
	return out, nil
}

func cleanPathList(paths []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = filepath.Clean(path)
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	return out
}
