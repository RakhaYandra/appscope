package group

import (
	"regexp"
	"strings"
)

var appScopeRe = regexp.MustCompile(`^app-(.+)\.scope$`)
var instanceIDRe = regexp.MustCompile(`^([0-9]+|[0-9a-f]{6,})$`)

var launcherNames = map[string]bool{
	"uwsm": true, "dbus-launch": true, "gio-launch-desktop": true, "gtk-launch": true,
	"gnome-session": true, "plasmashell": true,
}

func isLauncherName(name string) bool {
	n := strings.ToLower(name)
	if launcherNames[n] {
		return true
	}
	return strings.Contains(n, "launch")
}

var launcherPrefixes = map[string]bool{
	"gnome": true, "kde": true, "plasma": true, "flatpak": true,
	"wayland": true, "x11": true, "unity": true, "cinnamon": true, "xfce": true,
	"uwsm": true,
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
		name := normalizeScopeInner(m[1])
		if name == "" {
			continue
		}
		if isLauncherName(name) {
			// scope named after the launcher, not the app (e.g. uwsm/gtk-launch);
			// let desktop-file or tree resolution identify the real app
			return "", false
		}
		return name, true
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
