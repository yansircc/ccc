package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// discoverClaude finds the real claude binary.
// Priority: 1) ~/.local/bin/claude symlink  2) latest in versions/  3) "claude" in PATH
func discoverClaude(home string) string {
	self, _ := os.Executable()
	selfReal, _ := filepath.EvalSymlinks(self)

	// 1. Follow Claude's managed symlink
	managed := filepath.Join(home, ".local", "bin", "claude")
	if target, err := filepath.EvalSymlinks(managed); err == nil {
		if isUsableClaudeBinary(target, selfReal) {
			return target
		}
	}

	// 2. Scan versions directory, pick latest valid binary
	versionsDir := filepath.Join(home, ".local", "share", "claude", "versions")
	if entries, err := os.ReadDir(versionsDir); err == nil {
		sort.Slice(entries, func(i, j int) bool {
			return compareVersion(entries[i].Name(), entries[j].Name()) > 0
		})
		for _, e := range entries {
			path := filepath.Join(versionsDir, e.Name())
			if !e.Type().IsRegular() {
				continue
			}
			if isUsableClaudeBinary(path, selfReal) {
				return path
			}
		}
	}

	// 3. "claude" in PATH (skip self)
	if p, err := exec.LookPath("claude"); err == nil {
		pReal, _ := filepath.EvalSymlinks(p)
		if selfReal != pReal {
			return p
		}
	}

	fmt.Fprintln(os.Stderr, "Error: cannot find claude binary")
	os.Exit(1)
	return ""
}

func isUsableClaudeBinary(path string, selfReal string) bool {
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	if selfReal != "" && realPath == selfReal {
		return false
	}
	info, err := os.Stat(realPath)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Size() > 1_000_000
}

func compareVersion(a, b string) int {
	ap := strings.Split(strings.TrimPrefix(a, "v"), ".")
	bp := strings.Split(strings.TrimPrefix(b, "v"), ".")
	n := len(ap)
	if len(bp) > n {
		n = len(bp)
	}
	for i := 0; i < n; i++ {
		ai := parseVersionPart(ap, i)
		bi := parseVersionPart(bp, i)
		if ai > bi {
			return 1
		}
		if ai < bi {
			return -1
		}
	}
	return strings.Compare(a, b)
}

func parseVersionPart(parts []string, idx int) int {
	if idx >= len(parts) {
		return 0
	}
	n, err := strconv.Atoi(parts[idx])
	if err != nil {
		return 0
	}
	return n
}
