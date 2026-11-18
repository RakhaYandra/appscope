package group

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/RakhaYandra/appscope/internal/engine"
)

func TestScopeAppName(t *testing.T) {
	cases := []struct {
		cgroup string
		want   string
		ok     bool
	}{
		{"0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-gnome-firefox-1234.scope\n", "firefox", true},
		{"0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-flatpak-org.mozilla.firefox-5678.scope\n", "firefox", true},
		{"0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-kde-org.kde.kate-42.scope\n", "kate", true},
		{"0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-org.gnome.TextEditor-99.scope\n", "TextEditor", true},
		{"0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-Hyprland-gtk\\x2dlaunch-b23018c3.scope\n", "Hyprland-gtk-launch", true},
		{"0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-gnome-my-app-deadbeef.scope\n", "my-app", true},
		{"0::/user.slice/user-1000.slice/session.slice/wayland-wm@wayland-0.service\n", "", false},
		{"0::/init.scope\n", "", false},
		{"0::/user.slice\n", "", false},
	}
	for _, tc := range cases {
		got, ok := scopeAppName(tc.cgroup)
		if ok != tc.ok {
			t.Errorf("cgroup %q: ok = %v, want %v", tc.cgroup, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("cgroup %q: name = %q, want %q", tc.cgroup, got, tc.want)
		}
	}
}

func TestDesktopIndexLookup(t *testing.T) {
	dir := t.TempDir()
	desktop := "[Desktop Entry]\nName=Firefox\nExec=/usr/lib/firefox/firefox %u\nType=Application\n\n[Desktop Action new-window]\nName=New Window\nExec=/usr/lib/firefox/firefox --new-window\n"
	if err := os.WriteFile(filepath.Join(dir, "firefox.desktop"), []byte(desktop), 0o644); err != nil {
		t.Fatal(err)
	}
	idx := buildDesktopIndex([]string{dir})
	name, ok := idx.lookup("/usr/lib/firefox/firefox")
	if !ok {
		t.Fatal("lookup failed")
	}
	if name != "Firefox" {
		t.Errorf("name = %q, want Firefox", name)
	}
	if _, ok := idx.lookup("/usr/bin/definitely-not-indexed"); ok {
		t.Error("unexpected match for unindexed binary")
	}
}

func proc(pid, ppid int, comm string) engine.ProcessMetrics {
	return engine.ProcessMetrics{PID: pid, PPID: ppid, Comm: comm}
}

func TestGrouperAggregatesByCgroupScope(t *testing.T) {
	root := t.TempDir()
	for pid, scope := range map[int]string{100: "app-gnome-firefox-1.scope", 101: "app-gnome-firefox-1.scope", 200: "app-gnome-code-2.scope"} {
		dir := filepath.Join(root, "proc", itoa(pid))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		cgroup := "0::/user.slice/user-1000.slice/app.slice/" + scope + "\n"
		if err := os.WriteFile(filepath.Join(dir, "cgroup"), []byte(cgroup), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	procs := []engine.ProcessMetrics{
		{PID: 100, PPID: 1, Comm: "firefox", CPUPercent: 10, MemoryRSSBytes: 1000, MemoryVMSBytes: 5000, DiskReadBps: 100, DiskWriteBps: 50},
		{PID: 101, PPID: 100, Comm: "Web Content", CPUPercent: 5, MemoryRSSBytes: 2000, MemoryVMSBytes: 6000, DiskReadBps: 200, DiskWriteBps: 60},
		{PID: 200, PPID: 1, Comm: "code", CPUPercent: 3, MemoryRSSBytes: 3000, MemoryVMSBytes: 7000, DiskReadBps: 300, DiskWriteBps: 70},
	}
	g := &Grouper{Root: root}
	got := g.Group(procs)
	if len(got) != 2 {
		t.Fatalf("apps = %d, want 2 (%v)", len(got), got)
	}
	byName := map[string]int{}
	for i, a := range got {
		byName[a.AppName] = i
	}
	ff := got[byName["firefox"]]
	if ff.ProcessCount != 2 || len(ff.PIDs) != 2 {
		t.Errorf("firefox count/pids = %d/%v", ff.ProcessCount, ff.PIDs)
	}
	if ff.CPUPercent != 15 {
		t.Errorf("firefox cpu = %f, want 15", ff.CPUPercent)
	}
	if ff.MemoryRSSBytes != 3000 || ff.MemoryVMSBytes != 11000 {
		t.Errorf("firefox mem = %d/%d", ff.MemoryRSSBytes, ff.MemoryVMSBytes)
	}
	if ff.DiskReadBps != 300 || ff.DiskWriteBps != 110 {
		t.Errorf("firefox io = %d/%d", ff.DiskReadBps, ff.DiskWriteBps)
	}
}

func TestGrouperFallsBackToDesktopMatch(t *testing.T) {
	root := t.TempDir()
	appsDir := t.TempDir()
	binDir := t.TempDir()
	bin := filepath.Join(binDir, "mybrowser")
	if err := os.WriteFile(bin, []byte("#!/bin/true\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	desktop := "[Desktop Entry]\nName=MyBrowser\nExec=" + bin + " %u\nType=Application\n"
	if err := os.WriteFile(filepath.Join(appsDir, "mybrowser.desktop"), []byte(desktop), 0o644); err != nil {
		t.Fatal(err)
	}
	pidDir := filepath.Join(root, "proc", "300")
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pidDir, "cgroup"), []byte("0::/user.slice\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bin, filepath.Join(pidDir, "exe")); err != nil {
		t.Fatal(err)
	}

	g := &Grouper{Root: root, DesktopDirs: []string{appsDir}}
	got := g.Group([]engine.ProcessMetrics{proc(300, 1, "mybrowser")})
	if len(got) != 1 || got[0].AppName != "MyBrowser" {
		t.Fatalf("apps = %+v, want MyBrowser via desktop match", got)
	}
}

func TestGrouperFallsBackToPIDTree(t *testing.T) {
	root := t.TempDir()
	procs := []engine.ProcessMetrics{
		proc(400, 1, "ghostapp"),
		proc(401, 400, "worker"),
		proc(402, 401, "helper"),
	}
	g := &Grouper{Root: root, DesktopDirs: []string{t.TempDir()}}
	got := g.Group(procs)
	if len(got) != 1 {
		t.Fatalf("apps = %d, want 1: %+v", len(got), got)
	}
	if got[0].AppName != "ghostapp" {
		t.Errorf("name = %q, want ghostapp (tree root comm)", got[0].AppName)
	}
	if got[0].ProcessCount != 3 {
		t.Errorf("count = %d, want 3", got[0].ProcessCount)
	}
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
