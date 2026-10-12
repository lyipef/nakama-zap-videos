package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

var cleanupMu sync.Mutex
var cleanups []func()

func registerCleanup(f func()) { cleanupMu.Lock(); cleanups = append(cleanups, f); cleanupMu.Unlock() }
func runCleanups() {
	cleanupMu.Lock()
	defer cleanupMu.Unlock()
	for _, f := range cleanups {
		f()
	}
	cleanups = nil
}

// Temperatura da GPU NVIDIA via nvidia-smi (vem junto com o driver). -1 se não houver.
func gpuTemp() int {
	if v, err := strconv.Atoi(os.Getenv("ZAPTAKU_FAKE_GPU")); err == nil { // usado em testes
		return v
	}
	p := nvidiaSmi()
	if p == "" {
		return -1
	}
	cmd := exec.Command(p, "--query-gpu=temperature.gpu", "--format=csv,noheader,nounits")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return -1
	}
	best := -1
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if v, err := strconv.Atoi(strings.TrimSpace(l)); err == nil && v > best {
			best = v
		}
	}
	return best
}

func gpuInfo() string {
	p := nvidiaSmi()
	if p == "" {
		return ""
	}
	cmd := exec.Command(p, "--query-gpu=name,driver_version,temperature.gpu", "--format=csv,noheader")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.Split(string(out), "\n")[0])
}
