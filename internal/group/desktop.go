package group

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type desktopIndex struct {
	byExec map[string]string
}

func buildDesktopIndex(dirs []string) *desktopIndex {
	idx := &desktopIndex{byExec: map[string]string{}}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".desktop") {
				continue
			}
			name, exec := parseDesktopEntry(filepath.Join(dir, e.Name()))
			if exec == "" {
				continue
			}
			base := filepath.Base(exec)
			if _, exists := idx.byExec[base]; exists {
				continue
			}
			if name == "" {
				name = strings.TrimSuffix(e.Name(), ".desktop")
			}
			idx.byExec[base] = name
		}
	}
	return idx
}

func parseDesktopEntry(path string) (name, exec string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()

	inEntry := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inEntry = line == "[Desktop Entry]"
			continue
		}
		if !inEntry {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "Name":
			if name == "" {
				name = val
			}
		case "Exec":
			if exec == "" {
				fields := strings.Fields(val)
				if len(fields) > 0 {
					exec = strings.TrimLeft(fields[0], `"'`)
					exec = strings.TrimRight(exec, `"'`)
				}
			}
		}
	}
	return name, exec
}

func (d *desktopIndex) lookup(exePath string) (string, bool) {
	if d == nil {
		return "", false
	}
	base := filepath.Base(exePath)
	name, ok := d.byExec[base]
	return name, ok
}
