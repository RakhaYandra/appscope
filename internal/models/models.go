package models

import "time"

type SystemMetrics struct {
	Timestamp        time.Time `json:"timestamp"`
	CPUPercent       float64   `json:"cpu_percent"`
	MemoryUsedBytes  uint64    `json:"memory_used_bytes"`
	MemoryTotalBytes uint64    `json:"memory_total_bytes"`
	DiskReadBps      uint64    `json:"disk_read_bps"`
	DiskWriteBps     uint64    `json:"disk_write_bps"`
	ActiveProcesses  int       `json:"active_processes"`
}

type AppMetrics struct {
	AppName        string  `json:"app_name"`
	PIDs           []int   `json:"pids"`
	ProcessCount   int     `json:"process_count"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryRSSBytes uint64  `json:"memory_rss_bytes"`
	MemoryVMSBytes uint64  `json:"memory_vms_bytes"`
	DiskReadBps    uint64  `json:"disk_read_bps"`
	DiskWriteBps   uint64  `json:"disk_write_bps"`
}

type IPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
	ID      uint64                 `json:"id"`
}

type IPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *IPCError   `json:"error,omitempty"`
	ID      uint64      `json:"id"`
}

type IPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
