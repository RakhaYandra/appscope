package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RakhaYandra/appscope/internal/models"
)

type fakeSource struct {
	snap Snapshot
}

func (f fakeSource) Snapshot() Snapshot { return f.snap }

func testSnapshot() Snapshot {
	return Snapshot{
		Timestamp: time.Date(2026, 11, 18, 21, 45, 0, 0, time.UTC),
		System: models.SystemMetrics{
			CPUPercent:       42.5,
			MemoryUsedBytes:  9 * 1024 * 1024 * 1024,
			MemoryTotalBytes: 16 * 1024 * 1024 * 1024,
			DiskReadBps:      12 * 1024 * 1024,
			DiskWriteBps:     4 * 1024 * 1024,
			ActiveProcesses:  312,
		},
		Apps: []models.AppMetrics{
			{AppName: "firefox", ProcessCount: 5, CPUPercent: 18.2, MemoryRSSBytes: 2576980377, DiskReadBps: 14200000},
			{AppName: "code", ProcessCount: 8, CPUPercent: 8.5, MemoryRSSBytes: 1932735283, DiskReadBps: 2100000},
		},
	}
}

func startTestServer(t *testing.T, src Source) (string, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "appscope.sock")
	srv, err := NewServer(path, src)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve() }()
	stop := func() { _ = srv.Close() }
	t.Cleanup(stop)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", path)
		if err == nil {
			_ = conn.Close()
			return path, stop
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("server did not start")
	return path, stop
}

func callRPC(t *testing.T, path, method string, params map[string]interface{}) (models.IPCResponse, error) {
	t.Helper()
	conn, err := net.Dial("unix", path)
	if err != nil {
		return models.IPCResponse{}, err
	}
	defer func() { _ = conn.Close() }()
	req := models.IPCRequest{JSONRPC: "2.0", Method: method, Params: params, ID: 1}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return models.IPCResponse{}, err
	}
	var resp models.IPCResponse
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return models.IPCResponse{}, fmt.Errorf("read response: %w", err)
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		return models.IPCResponse{}, err
	}
	return resp, nil
}

func TestSocketPermissions0600(t *testing.T) {
	path, _ := startTestServer(t, fakeSource{testSnapshot()})
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("socket perm = %o, want 600", info.Mode().Perm())
	}
}

func TestPing(t *testing.T) {
	path, _ := startTestServer(t, fakeSource{testSnapshot()})
	resp, err := callRPC(t, path, "ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("error = %+v", resp.Error)
	}
}

func TestGetTopApps(t *testing.T) {
	path, _ := startTestServer(t, fakeSource{testSnapshot()})
	resp, err := callRPC(t, path, "appscope.getTopApps", map[string]interface{}{"limit": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("error = %+v", resp.Error)
	}
	result := resp.Result.(map[string]interface{})
	apps := result["applications"].([]interface{})
	if len(apps) != 1 {
		t.Fatalf("apps = %d, want 1 (limit)", len(apps))
	}
	first := apps[0].(map[string]interface{})
	if first["app_name"] != "firefox" {
		t.Errorf("first app = %v, want firefox (sorted by cpu desc)", first["app_name"])
	}
	system := result["system"].(map[string]interface{})
	if system["cpu_percent"].(float64) != 42.5 {
		t.Errorf("system cpu = %v", system["cpu_percent"])
	}
}

func TestGetAppByName(t *testing.T) {
	path, _ := startTestServer(t, fakeSource{testSnapshot()})
	resp, err := callRPC(t, path, "appscope.getApp", map[string]interface{}{"name": "code"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("error = %+v", resp.Error)
	}
	app := resp.Result.(map[string]interface{})
	if app["app_name"] != "code" {
		t.Errorf("app = %v", app["app_name"])
	}
	resp, err = callRPC(t, path, "appscope.getApp", map[string]interface{}{"name": "nonexistent"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for unknown app")
	}
}

func TestUnknownMethod(t *testing.T) {
	path, _ := startTestServer(t, fakeSource{testSnapshot()})
	resp, err := callRPC(t, path, "no.such.method", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Errorf("error = %+v, want code -32601", resp.Error)
	}
}

func TestMalformedJSON(t *testing.T) {
	path, _ := startTestServer(t, fakeSource{testSnapshot()})
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte("{not json\n")); err != nil {
		t.Fatal(err)
	}
	var resp models.IPCResponse
	_ = json.NewDecoder(conn).Decode(&resp)
	if resp.Error == nil || resp.Error.Code != -32700 {
		t.Errorf("error = %+v, want code -32700", resp.Error)
	}
}
