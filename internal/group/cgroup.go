package group

import (
	"regexp"
	"strings"
)

var appScopeRe = regexp.MustCompile(`^app-(.+)\.scope$`)
var instanceIDRe = regexp.MustCompile(`^([0-9]+|[0-9a-f]{6,})$`)

var launcherPrefixes = map[string]bool{
	"gnome": true, "kde": true, "plasma": true, "flatpak": true,
	"wayland": true, "x11": true, "unity": true, "cinnamon": true, "xfce": true,
}

func scopeAppName(cgroupContent string) (string, bool) {
	for _, line := range strings.Split(cgroupContent, "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		path := strings.ReplaceAll(parts[2], `\x2d`, "-")
		base := path[strings.LastIndex(path, "/")+1:]
		m := appScopeRe.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		if name := normalizeScopeInner(m[1]); name != "" {
			return name, true
		}
	}
	return "", false
}

func normalizeScopeInner(inner string) string {
	tokens := strings.Split(inner, "-")
	end := len(tokens)
	for end > 0 && instanceIDRe.MatchString(tokens[end-1]) {
		end--
	}
	tokens = tokens[:end]
	for len(tokens) > 0 && launcherPrefixes[strings.ToLower(tokens[0])] {
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return ""
	}
	if len(tokens) == 1 {
		last := tokens[0]
		if i := strings.LastIndex(last, "."); i >= 0 {
			last = last[i+1:]
		}
		return last
	}
	return strings.Join(tokens, "-")
}
