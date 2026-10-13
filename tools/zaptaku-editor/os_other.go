//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func prepareFFmpeg(tmp string) (string, error) { return exec.LookPath("ffmpeg") }
func hideWindow(cmd *exec.Cmd)                 {}
func lowPriority(cmd *exec.Cmd, on bool)       {}
func pickFolder(title string) string           { return "" }
func openFolder(p string)                      {}
func openApp(url string)                       { fmt.Println("Interface:", url) }
func shutdownPC()                              { fmt.Println("(teste) desligaria o PC") }
func fatalBox(msg string)                      { fmt.Println(msg); os.Exit(1) }
func suspendProcess(p *os.Process)             { p.Signal(syscall.SIGSTOP) }
func resumeProcess(p *os.Process)              { p.Signal(syscall.SIGCONT) }

func nvidiaSmi() string { p, _ := exec.LookPath("nvidia-smi"); return p }

func cpuTemp() int {
	if v, ok := readFakeInt("ZAPTAKU_FAKE_CPU"); ok {
		return v
	}
	best := -1
	files, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	for _, f := range files {
		if b, err := os.ReadFile(f); err == nil {
			if v, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && v/1000 > best {
				best = v / 1000
			}
		}
	}
	return best
}

var lastIdle, lastTotal uint64

func cpuUsage() int {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	f := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])[1:]
	var total, idle uint64
	for i, s := range f {
		v, _ := strconv.ParseUint(s, 10, 64)
		total += v
		if i == 3 || i == 4 {
			idle += v
		}
	}
	di, dt := idle-lastIdle, total-lastTotal
	lastIdle, lastTotal = idle, total
	if dt == 0 || dt == total {
		return 0
	}
	return int(100 * (1 - float64(di)/float64(dt)))
}

func ramGB() (float64, float64) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	var tot, avail float64
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseFloat(f[1], 64)
		switch f[0] {
		case "MemTotal:":
			tot = v
		case "MemAvailable:":
			avail = v
		}
	}
	return (tot - avail) / 1048576, tot / 1048576
}
