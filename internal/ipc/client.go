package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/RakhaYandra/appscope/internal/models"
)

func DefaultSocketPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "appscope.sock")
	}
	return filepath.Join("/run/user", fmt.Sprint(os.Getuid()), "appscope.sock")
}

func Call(socketPath, method string, params map[string]interface{}) (map[string]interface{}, error) {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", socketPath, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	req := models.IPCRequest{JSONRPC: "2.0", Method: method, Params: params, ID: 1}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	var resp models.IPCResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected result type %T", resp.Result)
	}
	return result, nil
}
