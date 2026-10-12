//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func prepareFFmpeg(tmp string) (string, error) { return exec.LookPath("ffmpeg") }
func setupConsole()                            {}
func hideWindow(cmd *exec.Cmd)                 {}
func pickFolder() string                       { return "" }
func openFolder(p string)                      {}
func suspendProcess(p *os.Process)             { p.Signal(syscall.SIGSTOP) }
func resumeProcess(p *os.Process)              { p.Signal(syscall.SIGCONT) }

func nvidiaSmi() string {
	p, _ := exec.LookPath("nvidia-smi")
	return p
}

func cpuTemp() int {
	if f := os.Getenv("ZAPTAKU_FAKE_CPU_FILE"); f != "" { // usado em testes
		b, _ := os.ReadFile(f)
		if v, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
			return v
		}
	}
	best := -1
	files, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if v, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && v/1000 > best {
			best = v / 1000
		}
	}
	return best
}
