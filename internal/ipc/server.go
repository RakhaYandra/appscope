package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/RakhaYandra/appscope/internal/models"
)

type Snapshot struct {
	Timestamp time.Time
	System    models.SystemMetrics
	Apps      []models.AppMetrics
}

type Source interface {
	Snapshot() Snapshot
}

type Server struct {
	path     string
	listener net.Listener
	src      Source

	wg     sync.WaitGroup
	closed chan struct{}
	once   sync.Once
}

func NewServer(path string, src Source) (*Server, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create socket dir: %w", err)
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("chmod socket: %w", err)
	}
	return &Server{path: path, listener: ln, src: src, closed: make(chan struct{})}, nil
}

func (s *Server) Path() string { return s.path }

func (s *Server) Serve() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return nil
			default:
				return fmt.Errorf("accept: %w", err)
			}
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handle(conn)
		}()
	}
}

func (s *Server) Close() error {
	var err error
	s.once.Do(func() {
		close(s.closed)
		err = s.listener.Close()
		s.wg.Wait()
		_ = os.Remove(s.path)
	})
	return err
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return
	}

	var req models.IPCRequest
	if err := json.Unmarshal(line, &req); err != nil {
		s.writeError(conn, -32700, "parse error", 0)
		return
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		s.writeError(conn, -32600, "invalid request", req.ID)
		return
	}

	result, rpcErr := s.dispatch(req)
	if rpcErr != nil {
		s.writeError(conn, rpcErr.code, rpcErr.message, req.ID)
		return
	}
	s.write(conn, models.IPCResponse{JSONRPC: "2.0", Result: result, ID: req.ID})
}

type rpcError struct {
	code    int
	message string
}

func (e *rpcError) Error() string { return e.message }

func (s *Server) dispatch(req models.IPCRequest) (interface{}, *rpcError) {
	switch req.Method {
	case "ping":
		return map[string]interface{}{"pong": true}, nil
	case "appscope.getSystem":
		snap := s.src.Snapshot()
		return systemPayload(snap), nil
	case "appscope.getTopApps":
		return s.topAppsBody(req.Params), nil
	case "appscope.getApp":
		return s.getApp(req.Params)
	default:
		return nil, &rpcError{code: -32601, message: "method not found: " + req.Method}
	}
}

func systemPayload(snap Snapshot) map[string]interface{} {
	return map[string]interface{}{
		"cpu_percent":      snap.System.CPUPercent,
		"memory_used_gb":   float64(snap.System.MemoryUsedBytes) / (1024 * 1024 * 1024),
		"memory_total_gb":  float64(snap.System.MemoryTotalBytes) / (1024 * 1024 * 1024),
		"disk_read_mbps":   float64(snap.System.DiskReadBps) / (1024 * 1024),
		"disk_write_mbps":  float64(snap.System.DiskWriteBps) / (1024 * 1024),
		"active_processes": snap.System.ActiveProcesses,
	}
}

func (s *Server) topAppsBody(params map[string]interface{}) map[string]interface{} {
	snap := s.src.Snapshot()
	limit := 10
	if v, ok := params["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	sortKey := "cpu"
	if v, ok := params["sort"].(string); ok {
		sortKey = v
	}
	apps := append([]models.AppMetrics{}, snap.Apps...)
	switch sortKey {
	case "memory":
		sort.Slice(apps, func(i, j int) bool { return apps[i].MemoryRSSBytes > apps[j].MemoryRSSBytes })
	case "disk":
		sort.Slice(apps, func(i, j int) bool {
			return apps[i].DiskReadBps+apps[i].DiskWriteBps > apps[j].DiskReadBps+apps[j].DiskWriteBps
		})
	default:
		sort.Slice(apps, func(i, j int) bool { return apps[i].CPUPercent > apps[j].CPUPercent })
	}
	if len(apps) > limit {
		apps = apps[:limit]
	}

	appPayloads := make([]map[string]interface{}, 0, len(apps))
	for _, a := range apps {
		appPayloads = append(appPayloads, appPayload(a))
	}
	return map[string]interface{}{
		"timestamp":    snap.Timestamp.UTC().Format(time.RFC3339),
		"system":       systemPayload(snap),
		"applications": appPayloads,
	}
}

func appPayload(a models.AppMetrics) map[string]interface{} {
	return map[string]interface{}{
		"app_name":         a.AppName,
		"pids":             a.PIDs,
		"process_count":    a.ProcessCount,
		"cpu_percent":      a.CPUPercent,
		"memory_rss_bytes": a.MemoryRSSBytes,
		"memory_vms_bytes": a.MemoryVMSBytes,
		"disk_read_bps":    a.DiskReadBps,
		"disk_write_bps":   a.DiskWriteBps,
	}
}

func (s *Server) getApp(params map[string]interface{}) (interface{}, *rpcError) {
	snap := s.src.Snapshot()
	name, _ := params["name"].(string)
	if pidF, ok := params["pid"].(float64); ok {
		pid := int(pidF)
		for _, a := range snap.Apps {
			for _, p := range a.PIDs {
				if p == pid {
					return appPayload(a), nil
				}
			}
		}
		return nil, &rpcError{code: -32004, message: fmt.Sprintf("no app found for pid %d", pid)}
	}
	for _, a := range snap.Apps {
		if a.AppName == name {
			return appPayload(a), nil
		}
	}
	return nil, &rpcError{code: -32004, message: "app not found: " + name}
}

func (s *Server) write(conn net.Conn, resp models.IPCResponse) {
	b, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_, _ = conn.Write(append(b, '\n'))
}

func (s *Server) writeError(conn net.Conn, code int, message string, id uint64) {
	s.write(conn, models.IPCResponse{JSONRPC: "2.0", Error: &models.IPCError{Code: code, Message: message}, ID: id})
}
