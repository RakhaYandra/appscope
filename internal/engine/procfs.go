package engine

import (
	"fmt"
	"strconv"
	"strings"
)

func parseProcStat(content string) (total, idle uint64, err error) {
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line[len("cpu "):])
		if len(fields) < 8 {
			return 0, 0, fmt.Errorf("parse /proc/stat: got %d fields, want >= 8", len(fields))
		}
		vals := make([]uint64, 0, len(fields))
		for _, f := range fields {
			v, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				return 0, 0, fmt.Errorf("parse /proc/stat field %q: %w", f, err)
			}
			vals = append(vals, v)
		}
		for i, v := range vals[:8] {
			total += v
			if i == 3 || i == 4 {
				idle += v
			}
		}
		return total, idle, nil
	}
	return 0, 0, fmt.Errorf("parse /proc/stat: aggregate 'cpu' line not found")
}

type processStat struct {
	Comm           string
	State          string
	PPID           int
	UTime          uint64
	STime          uint64
	Threads        int
	StartTimeTicks uint64
}

func parseProcessStat(pid int, line string) (processStat, error) {
	var st processStat
	open := strings.Index(line, "(")
	closeIdx := strings.LastIndex(line, ")")
	if open < 0 || closeIdx < 0 || closeIdx < open {
		return st, fmt.Errorf("parse /proc/%d/stat: malformed comm field", pid)
	}
	st.Comm = line[open+1 : closeIdx]

	rest := strings.Fields(line[closeIdx+1:])
	// rest[0] is field 3 (state); utime=field 14 → index 11, stime=field 15 → index 12, starttime=field 22 → index 19
	if len(rest) < 20 {
		return st, fmt.Errorf("parse /proc/%d/stat: got %d trailing fields, want >= 20", pid, len(rest))
	}
	st.State = rest[0]
	ppid, err := strconv.Atoi(rest[1])
	if err != nil {
		return st, fmt.Errorf("parse /proc/%d/stat ppid: %w", pid, err)
	}
	st.PPID = ppid
	st.StartTimeTicks, err = strconv.ParseUint(rest[19], 10, 64)
	if err != nil {
		return st, fmt.Errorf("parse /proc/%d/stat starttime: %w", pid, err)
	}
	st.UTime, err = strconv.ParseUint(rest[11], 10, 64)
	if err != nil {
		return st, fmt.Errorf("parse /proc/%d/stat utime: %w", pid, err)
	}
	st.STime, err = strconv.ParseUint(rest[12], 10, 64)
	if err != nil {
		return st, fmt.Errorf("parse /proc/%d/stat stime: %w", pid, err)
	}
	st.Threads, err = strconv.Atoi(rest[17])
	if err != nil {
		return st, fmt.Errorf("parse /proc/%d/stat num_threads: %w", pid, err)
	}
	return st, nil
}

func parseProcessStatus(content string) (vmRSS, vmSize uint64, threads, uid int, err error) {
	uid = -1
	for _, line := range strings.Split(content, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(val), " kB"))
		switch key {
		case "VmRSS":
			kb, err := strconv.ParseUint(val, 10, 64)
			if err != nil {
				return 0, 0, 0, -1, fmt.Errorf("parse VmRSS: %w", err)
			}
			vmRSS = kb * 1024
		case "VmSize":
			kb, err := strconv.ParseUint(val, 10, 64)
			if err != nil {
				return 0, 0, 0, -1, fmt.Errorf("parse VmSize: %w", err)
			}
			vmSize = kb * 1024
		case "Threads":
			n, err := strconv.Atoi(strings.TrimSpace(val))
			if err != nil {
				return 0, 0, 0, -1, fmt.Errorf("parse Threads: %w", err)
			}
			threads = n
		case "Uid":
			fields := strings.Fields(val)
			if len(fields) > 0 {
				n, err := strconv.Atoi(fields[0])
				if err != nil {
					return 0, 0, 0, -1, fmt.Errorf("parse Uid: %w", err)
				}
				uid = n
			}
		}
	}
	return vmRSS, vmSize, threads, uid, nil
}

func parseProcessIO(content string) (readBytes, writeBytes uint64, err error) {
	for _, line := range strings.Split(content, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v, err := strconv.ParseUint(strings.TrimSpace(val), 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("parse /proc io %q: %w", key, err)
		}
		switch key {
		case "read_bytes":
			readBytes = v
		case "write_bytes":
			writeBytes = v
		}
	}
	return readBytes, writeBytes, nil
}

func parseMeminfo(content string) (total, available uint64, err error) {
	for _, line := range strings.Split(content, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		kb, err := strconv.ParseUint(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(val), " kB")), 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("parse meminfo %q: %w", key, err)
		}
		switch key {
		case "MemTotal":
			total = kb * 1024
		case "MemAvailable":
			available = kb * 1024
		}
	}
	if total == 0 {
		return 0, 0, fmt.Errorf("parse meminfo: MemTotal not found")
	}
	return total, available, nil
}

func parseBtime(content string) (uint64, error) {
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "btime ") {
			continue
		}
		v, err := strconv.ParseUint(strings.TrimSpace(line[len("btime "):]), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse btime: %w", err)
		}
		return v, nil
	}
	return 0, fmt.Errorf("parse btime: btime line not found")
}
