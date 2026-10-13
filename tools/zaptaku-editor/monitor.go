package main

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type GPUStat struct {
	OK       bool    `json:"ok"`
	Name     string  `json:"name"`
	Temp     int     `json:"temp"` // -1 = sem leitura
	Util     int     `json:"util"`
	MemUsed  int     `json:"memUsed"`  // MiB
	MemTotal int     `json:"memTotal"` // MiB
	Power    float64 `json:"power"`    // W
	Fan      int     `json:"fan"`      // %, -1 = n/d
}

type CPUStat struct {
	Temp     int     `json:"temp"`
	Usage    int     `json:"usage"`
	RAMUsed  float64 `json:"ramUsed"` // GB
	RAMTotal float64 `json:"ramTotal"`
	Threads  int     `json:"threads"`
}

type Sample struct {
	T    int64 `json:"t"`
	GPUT int   `json:"gt"`
	CPUT int   `json:"ct"`
	GPUU int   `json:"gu"`
	CPUU int   `json:"cu"`
}

type Telemetry struct {
	GPU  GPUStat  `json:"gpu"`
	CPU  CPUStat  `json:"cpu"`
	Hist []Sample `json:"hist"`
}

type teleStore struct {
	mu  sync.Mutex
	gpu GPUStat
	cpu CPUStat
	h   []Sample
}

var telemetry = &teleStore{gpu: GPUStat{Temp: -1, Fan: -1}, cpu: CPUStat{Temp: -1}}

func (t *teleStore) snapshot() Telemetry {
	t.mu.Lock()
	defer t.mu.Unlock()
	return Telemetry{GPU: t.gpu, CPU: t.cpu, Hist: append([]Sample(nil), t.h...)}
}

func atoiSafe(s string, def int) int {
	s = strings.TrimSpace(s)
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int(f + 0.5)
	}
	return def
}

func readFakeInt(env string) (int, bool) {
	f := os.Getenv(env)
	if f == "" {
		return 0, false
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return 0, false
	}
	return atoiSafe(string(b), -1), true
}

// nvidia-smi fica rodando e envia uma linha a cada 2s (mais leve que abrir um processo por leitura).
func gpuLoop() {
	if v, ok := readFakeInt("ZAPTAKU_FAKE_GPU"); ok { // teste
		for range time.Tick(2 * time.Second) {
			v, _ = readFakeInt("ZAPTAKU_FAKE_GPU")
			telemetry.mu.Lock()
			telemetry.gpu = GPUStat{OK: true, Name: "GPU de teste", Temp: v, Util: 60, MemUsed: 2100, MemTotal: 8188, Power: 82, Fan: 45}
			telemetry.mu.Unlock()
		}
		return
	}
	p := nvidiaSmi()
	if p == "" {
		return
	}
	for {
		cmd := exec.Command(p, "--query-gpu=name,temperature.gpu,utilization.gpu,memory.used,memory.total,power.draw,fan.speed",
			"--format=csv,noheader,nounits", "-l", "2")
		hideWindow(cmd)
		out, err := cmd.StdoutPipe()
		if err != nil || cmd.Start() != nil {
			return
		}
		registerCleanup(func() { cmd.Process.Kill() })
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			f := strings.Split(sc.Text(), ",")
			if len(f) < 7 {
				continue
			}
			g := GPUStat{OK: true, Name: strings.TrimSpace(f[0]), Temp: atoiSafe(f[1], -1), Util: atoiSafe(f[2], 0),
				MemUsed: atoiSafe(f[3], 0), MemTotal: atoiSafe(f[4], 0), Fan: atoiSafe(f[6], -1)}
			g.Power, _ = strconv.ParseFloat(strings.TrimSpace(f[5]), 64)
			telemetry.mu.Lock()
			telemetry.gpu = g
			telemetry.mu.Unlock()
		}
		cmd.Wait()
		time.Sleep(5 * time.Second)
	}
}

func sysLoop() {
	for range time.Tick(2 * time.Second) {
		c := CPUStat{Temp: cpuTemp(), Usage: cpuUsage(), Threads: numCPU()}
		c.RAMUsed, c.RAMTotal = ramGB()
		telemetry.mu.Lock()
		telemetry.cpu = c
		telemetry.h = append(telemetry.h, Sample{T: time.Now().Unix(), GPUT: telemetry.gpu.Temp, CPUT: c.Temp, GPUU: telemetry.gpu.Util, CPUU: c.Usage})
		if len(telemetry.h) > 150 { // ~5 minutos
			telemetry.h = telemetry.h[len(telemetry.h)-150:]
		}
		telemetry.mu.Unlock()
	}
}

// ---------- limpeza ao sair ----------

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
