// Zaptaku Editor: aplica os templates do @zaptaku em todos os vídeos de uma pasta.
// Abre uma interface (janela do Edge em modo app) servida localmente.
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed assets/loop_hud.mp4 assets/loop_forja.mp4 assets/overlay_hud.png assets/overlay_forja.png
var assets embed.FS

//go:embed ui
var uiFS embed.FS

const port = 47823

var lastPing atomic.Int64
var ready atomic.Bool
var readyErr atomic.Value

func numCPU() int { return runtime.NumCPU() }

func appDir() string {
	d, err := os.UserCacheDir()
	if err != nil {
		d = os.TempDir()
	}
	d = filepath.Join(d, "Zaptaku")
	os.MkdirAll(d, 0o755)
	return d
}

func cacheDir() string {
	d := filepath.Join(appDir(), "cache")
	os.MkdirAll(d, 0o755)
	return d
}

func assetKey(style string) string {
	a, _ := assets.ReadFile("assets/loop_" + style + ".mp4")
	b, _ := assets.ReadFile("assets/overlay_" + style + ".png")
	return strconv.Itoa(len(a)) + "-" + strconv.Itoa(len(b)) + "-v2"
}
func assetChanged(style string) bool {
	b, err := os.ReadFile(filepath.Join(cacheDir(), "key_"+style+".txt"))
	return err != nil || string(b) != assetKey(style)
}
func markAsset(style string) {
	os.WriteFile(filepath.Join(cacheDir(), "key_"+style+".txt"), []byte(assetKey(style)), 0o644)
}

var cfgMu sync.Mutex

func loadConfig() Config {
	cfg := defaultConfig()
	if b, err := os.ReadFile(filepath.Join(appDir(), "config.json")); err == nil {
		json.Unmarshal(b, &cfg)
	}
	return cfg
}
func saveConfig(c Config) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	b, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(filepath.Join(appDir(), "config.json"), b, 0o644)
}

func extractAssets(dir string) error {
	for _, n := range []string{"loop_hud.mp4", "loop_forja.mp4", "overlay_hud.png", "overlay_forja.png"} {
		b, err := assets.ReadFile("assets/" + n)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, n), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func readCfg(r *http.Request) (Config, error) {
	c := defaultConfig()
	err := json.NewDecoder(r.Body).Decode(&c)
	return c, err
}

func main() {
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil { // já está aberto: só mostra a janela
		openApp(url)
		return
	}
	defer runCleanups()

	tmp, _ := os.MkdirTemp("", "zaptaku-editor-")
	registerCleanup(func() { os.RemoveAll(tmp) })
	job.tmp = tmp
	job.cfg = loadConfig()
	go func() {
		if err := extractAssets(tmp); err != nil {
			readyErr.Store(err.Error())
			return
		}
		ff, err := prepareFFmpeg(tmp)
		if err != nil {
			readyErr.Store(err.Error())
			return
		}
		job.ffmpeg = ff
		ready.Store(true)
	}()
	go gpuLoop()
	go sysLoop()
	go thermalLoop()

	mux := http.NewServeMux()
	sub, _ := fs.Sub(uiFS, "ui")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		s := snapshot()
		writeJSON(w, map[string]any{"snap": s, "ready": ready.Load(), "readyErr": readyErr.Load(), "defaults": defaultConfig()})
	})
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "stream não suportado", 500)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			lastPing.Store(time.Now().Unix())
			b, _ := json.Marshal(map[string]any{"snap": snapshot(), "ready": ready.Load(), "readyErr": readyErr.Load()})
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-t.C:
			}
		}
	})
	mux.HandleFunc("/api/pick", func(w http.ResponseWriter, r *http.Request) {
		title := "Escolha a pasta com os vídeos para editar"
		if r.URL.Query().Get("kind") == "out" {
			title = "Escolha onde salvar os vídeos editados"
		}
		writeJSON(w, map[string]string{"path": pickFolder(title)})
	})
	mux.HandleFunc("/api/scan", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := readCfg(r)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		items, err := buildItems(cfg)
		if err != nil {
			writeJSON(w, map[string]any{"error": "Não consegui ler essa pasta. Confira o caminho."})
			return
		}
		saveConfig(cfg)
		writeJSON(w, map[string]any{"items": items, "out": cfg.outDir()})
	})
	mux.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := readCfg(r)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if !ready.Load() {
			writeJSON(w, map[string]any{"error": "O motor de vídeo ainda está sendo preparado. Tente de novo em alguns segundos."})
			return
		}
		if err := startJob(cfg); err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/pause", func(w http.ResponseWriter, r *http.Request) {
		pauseJob(r.URL.Query().Get("on") == "1")
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/cancel", func(w http.ResponseWriter, r *http.Request) {
		cancelJob()
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/open", func(w http.ResponseWriter, r *http.Request) {
		job.mu.Lock()
		cfg := job.cfg
		job.mu.Unlock()
		if p := r.URL.Query().Get("path"); p != "" {
			openFolder(p)
		} else {
			openFolder(cfg.outDir())
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true})
		go func() { time.Sleep(300 * time.Millisecond); cancelJob(); runCleanups(); os.Exit(0) }()
	})

	// fecha sozinho se a janela foi fechada e não há nada processando
	lastPing.Store(time.Now().Unix())
	go func() {
		for range time.Tick(5 * time.Second) {
			job.mu.Lock()
			running := job.running
			job.mu.Unlock()
			if !running && time.Now().Unix()-lastPing.Load() > 30 {
				runCleanups()
				os.Exit(0)
			}
		}
	}()

	go func() { time.Sleep(400 * time.Millisecond); openApp(url) }()
	if err := http.Serve(ln, mux); err != nil {
		fatalBox("Zaptaku Editor não conseguiu iniciar: " + err.Error())
	}
}
