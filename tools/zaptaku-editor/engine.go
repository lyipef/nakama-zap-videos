package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var videoExt = map[string]bool{".mp4": true, ".mov": true, ".mkv": true, ".webm": true, ".avi": true, ".m4v": true}

// Config: tudo que a interface permite escolher.
type Config struct {
	Folder    string `json:"folder"`
	Out       string `json:"out"`       // vazio = <pasta>/zaptaku_editados
	Mode      string `json:"mode"`      // aleatorio | hud | forja
	Quality   string `json:"quality"`   // 4k | 1080
	Engine    string `json:"engine"`    // auto | gpu | cpu
	GPUJobs   int    `json:"gpuJobs"`   // 0 = automático
	CPUJobs   int    `json:"cpuJobs"`   // -1 = automático
	Denoise   bool   `json:"denoise"`   // redução de ruído
	Sharpen   bool   `json:"sharpen"`   // nitidez (CAS)
	Color     bool   `json:"color"`     // realce de cor
	Volume    int    `json:"volume"`    // % do áudio original
	Normalize bool   `json:"normalize"` // loudness -14 LUFS
	MaxDur    int    `json:"maxDur"`    // segundos; 0 = sem limite
	Priority  string `json:"priority"`  // normal | baixa
	Rework    bool   `json:"rework"`    // refazer mesmo os prontos
	OnFinish  string `json:"onFinish"`  // nada | abrir | desligar
	GPUMax    int    `json:"gpuMax"`
	GPUResume int    `json:"gpuResume"`
	CPUMax    int    `json:"cpuMax"`
	CPUResume int    `json:"cpuResume"`
}

func defaultConfig() Config {
	return Config{Mode: "aleatorio", Quality: "4k", Engine: "auto", GPUJobs: 0, CPUJobs: -1,
		Denoise: true, Sharpen: true, Color: true, Volume: 100, Normalize: false, MaxDur: 0,
		Priority: "normal", OnFinish: "abrir", GPUMax: 83, GPUResume: 75, CPUMax: 90, CPUResume: 80}
}

func (c Config) outDir() string {
	if strings.TrimSpace(c.Out) != "" {
		return c.Out
	}
	return filepath.Join(c.Folder, "zaptaku_editados")
}

// ---------------- itens / estado ----------------

type Item struct {
	Idx     int     `json:"idx"`
	Name    string  `json:"name"`
	Src     string  `json:"-"`
	Base    string  `json:"-"`
	Dur     float64 `json:"dur"`
	Status  string  `json:"status"` // pronto | fila | rodando | ok | erro | cancelado
	Worker  string  `json:"worker"`
	Style   string  `json:"style"`
	Pct     float64 `json:"pct"`
	Out     string  `json:"out"`
	Secs    float64 `json:"secs"` // tempo gasto
	Err     string  `json:"err"`
	started time.Time
}

type Job struct {
	mu        sync.Mutex
	cfg       Config
	items     []*Item
	running   bool
	paused    bool // pausa manual
	hot       bool // pausa térmica
	hotReason string
	canceled  bool
	encoder   string
	plan      string
	startedAt time.Time
	finished  time.Time
	doneSecs  float64 // segundos de vídeo concluídos
	procs     map[int]*os.Process
	cond      *sync.Cond
	logs      []string
	ffmpeg    string
	tmp       string
}

var job = func() *Job { j := &Job{procs: map[int]*os.Process{}}; j.cond = sync.NewCond(&j.mu); return j }()

var logFile *os.File

func logf(format string, a ...any) {
	line := time.Now().Format("15:04:05 ") + fmt.Sprintf(format, a...)
	job.mu.Lock()
	job.logs = append(job.logs, line)
	if len(job.logs) > 500 {
		job.logs = job.logs[len(job.logs)-500:]
	}
	f := logFile
	job.mu.Unlock()
	if f != nil {
		fmt.Fprintln(f, line)
	}
}

// ---------------- varredura ----------------

func listVideos(folder string) ([]string, error) {
	ents, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	var vids []string
	for _, e := range ents {
		if !e.IsDir() && videoExt[strings.ToLower(filepath.Ext(e.Name()))] {
			vids = append(vids, filepath.Join(folder, e.Name()))
		}
	}
	sort.Strings(vids)
	return vids, nil
}

// vídeos (nome base) já prontos na pasta de saída, para a qualidade escolhida.
func scanDone(out, quality string, clean bool) map[string]string {
	done := map[string]string{}
	ents, err := os.ReadDir(out)
	if err != nil {
		return done
	}
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(n, ".part.mp4") {
			if clean {
				os.Remove(filepath.Join(out, n))
				logf("removida sobra incompleta: %s", n)
			}
			continue
		}
		if !strings.HasSuffix(strings.ToLower(n), ".mp4") {
			continue
		}
		if info, err := e.Info(); err != nil || info.Size() < 10_000 {
			continue
		}
		stem := strings.TrimSuffix(n, filepath.Ext(n))
		if (quality == "4k") != strings.HasSuffix(stem, "_4k") {
			continue
		}
		for _, tag := range []string{"_zaptaku_", "_nakama_"} {
			if i := strings.LastIndex(stem, tag); i > 0 {
				done[stem[:i]] = n
				break
			}
		}
	}
	return done
}

func buildItems(cfg Config) ([]*Item, error) {
	vids, err := listVideos(cfg.Folder)
	if err != nil {
		return nil, err
	}
	done := map[string]string{}
	if !cfg.Rework {
		done = scanDone(cfg.outDir(), cfg.Quality, false)
	}
	var items []*Item
	for i, v := range vids {
		base := strings.TrimSuffix(filepath.Base(v), filepath.Ext(v))
		it := &Item{Idx: i, Name: filepath.Base(v), Src: v, Base: base, Status: "fila"}
		if o, ok := done[base]; ok {
			it.Status, it.Out, it.Pct = "pronto", o, 100
		}
		items = append(items, it)
	}
	return items, nil
}

// ---------------- codificadores ----------------

type encoder struct {
	name, codec string
	gpu         bool
	pixfmt      string
	hwaccel     string
}

var cpuEncoder = encoder{"CPU (x264)", "libx264", false, "yuv420p", ""}

var gpuEncoders = []encoder{
	{"NVIDIA NVENC", "h264_nvenc", true, "nv12", "cuda"},
	{"Intel Quick Sync", "h264_qsv", true, "nv12", "auto"},
	{"AMD AMF", "h264_amf", true, "nv12", "auto"},
}

func encArgs(e encoder, quality string, threads int) []string {
	hq := quality == "4k"
	switch e.codec {
	case "h264_nvenc":
		cq, maxr, buf := "21", "20M", "40M"
		if hq {
			cq, maxr, buf = "19", "80M", "160M"
		}
		return []string{"-c:v", "h264_nvenc", "-preset", "p5", "-tune", "hq", "-profile:v", "high",
			"-rc", "vbr", "-cq", cq, "-b:v", "0", "-maxrate", maxr, "-bufsize", buf, "-spatial-aq", "1", "-temporal-aq", "1", "-rc-lookahead", "20"}
	case "h264_qsv":
		q := "22"
		if hq {
			q = "20"
		}
		return []string{"-c:v", "h264_qsv", "-preset", "medium", "-global_quality", q}
	case "h264_amf":
		qi, qp := "21", "23"
		if hq {
			qi, qp = "19", "21"
		}
		return []string{"-c:v", "h264_amf", "-quality", "quality", "-rc", "cqp", "-qp_i", qi, "-qp_p", qp}
	default:
		preset, crf := "veryfast", "20"
		if hq {
			preset, crf = "faster", "18"
		}
		a := []string{"-c:v", "libx264", "-preset", preset, "-crf", crf}
		if hq {
			a = append(a, "-profile:v", "high", "-level", "5.1")
		}
		return append(a, "-threads", strconv.Itoa(threads))
	}
}

func encoderTest(ffmpeg string, e encoder, quality string) error {
	size := "1080x1920"
	if quality == "4k" {
		size = "2160x3840"
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=black:s=" + size + ":d=0.5:r=30", "-vf", "format=" + e.pixfmt}
	args = append(args, encArgs(e, quality, 2)...)
	args = append(args, "-f", "null", "-")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	hideWindow(cmd)
	var buf bytes.Buffer
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(buf.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", strings.SplitN(msg, "\n", 2)[0])
	}
	return nil
}

func detectGPU(ffmpeg, quality string) (encoder, bool) {
	for _, e := range gpuEncoders {
		if err := encoderTest(ffmpeg, e, quality); err != nil {
			logf("GPU %s indisponível: %v", e.name, err)
			continue
		}
		logf("GPU disponível: %s", e.name)
		return e, true
	}
	return encoder{}, false
}

// ---------------- mídia ----------------

var reDur = regexp.MustCompile(`Duration: (\d+):(\d+):(\d+(?:\.\d+)?)`)
var reAudio = regexp.MustCompile(`Stream #\d+:\d+[^:]*: Audio: (\w+)`)

type probe struct {
	dur   float64
	audio string // codec do áudio, "" se não tiver
}

func probeMedia(ffmpeg, src string) probe {
	cmd := exec.Command(ffmpeg, "-hide_banner", "-i", src)
	hideWindow(cmd)
	var buf bytes.Buffer
	cmd.Stderr = &buf
	_ = cmd.Run()
	s := buf.String()
	p := probe{}
	if m := reDur.FindStringSubmatch(s); m != nil {
		h, _ := strconv.ParseFloat(m[1], 64)
		mi, _ := strconv.ParseFloat(m[2], 64)
		se, _ := strconv.ParseFloat(m[3], 64)
		p.dur = h*3600 + mi*60 + se
	}
	if m := reAudio.FindStringSubmatch(s); m != nil {
		p.audio = m[1]
	}
	return p
}

// Fundo e moldura em 4K são preparados UMA vez (Lanczos) e reaproveitados,
// em vez de ampliar o fundo quadro a quadro em cada vídeo.
func prepareAssets(cfg Config, styles []string) error {
	if cfg.Quality != "4k" {
		return nil
	}
	cache := cacheDir()
	for _, st := range styles {
		bg := filepath.Join(cache, "loop_"+st+"_2160.mp4")
		ov := filepath.Join(cache, "overlay_"+st+"_2160.png")
		if fi, err := os.Stat(bg); err != nil || fi.Size() < 1000 || assetChanged(st) {
			logf("preparando fundo 4K (%s)...", st)
			cmd := exec.Command(job.ffmpeg, "-y", "-hide_banner", "-loglevel", "error", "-i", filepath.Join(job.tmp, "loop_"+st+".mp4"),
				"-vf", "scale=2160:3840:flags=lanczos,setsar=1", "-c:v", "libx264", "-preset", "veryfast", "-crf", "12", "-g", "15", "-pix_fmt", "yuv420p", "-an", bg)
			hideWindow(cmd)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("fundo 4K: %v %s", err, out)
			}
			cmd = exec.Command(job.ffmpeg, "-y", "-hide_banner", "-loglevel", "error", "-i", filepath.Join(job.tmp, "overlay_"+st+".png"),
				"-vf", "scale=2160:3840:flags=lanczos", ov)
			hideWindow(cmd)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("moldura 4K: %v %s", err, out)
			}
			markAsset(st)
		}
	}
	return nil
}

func assetPaths(cfg Config, style string) (bg, ov string) {
	if cfg.Quality == "4k" {
		return filepath.Join(cacheDir(), "loop_"+style+"_2160.mp4"), filepath.Join(cacheDir(), "overlay_"+style+"_2160.png")
	}
	return filepath.Join(job.tmp, "loop_"+style+".mp4"), filepath.Join(job.tmp, "overlay_"+style+".png")
}

func buildFilter(cfg Config, pixfmt string) string {
	var clip []string
	W, Hc, Y := 1080, 608, 656
	if cfg.Quality == "4k" {
		W, Hc, Y = 2160, 1216, 1312
	}
	if cfg.Denoise {
		clip = append(clip, "hqdn3d=1.5:1.5:6:6") // na resolução original (mais barato)
	}
	clip = append(clip, fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease:flags=lanczos", W, Hc),
		fmt.Sprintf("pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=0x060610", W, Hc), "setsar=1")
	if cfg.Sharpen {
		clip = append(clip, "cas=0.45") // Contrast Adaptive Sharpening: mais leve e natural que unsharp
	}
	if cfg.Color {
		clip = append(clip, "eq=contrast=1.03:saturation=1.08")
	}
	clip = append(clip, "fps=30")
	return "[1:v]" + strings.Join(clip, ",") + "[clip];" +
		fmt.Sprintf("[0:v][clip]overlay=0:%d:shortest=1[b];", Y) +
		"[b][2:v]overlay=0:0,format=" + pixfmt + "[v]"
}

func audioArgs(cfg Config, p probe) []string {
	if p.audio == "" {
		return nil
	}
	var af []string
	if cfg.Volume != 100 {
		af = append(af, fmt.Sprintf("volume=%.2f", float64(cfg.Volume)/100))
	}
	if cfg.Normalize {
		af = append(af, "loudnorm=I=-14:TP=-1.5:LRA=11")
	}
	if len(af) == 0 && p.audio == "aac" && cfg.MaxDur == 0 {
		return []string{"-c:a", "copy"} // já está em AAC: copia sem reprocessar
	}
	a := []string{"-c:a", "aac", "-b:a", "192k", "-ar", "48000"}
	if len(af) > 0 {
		a = append([]string{"-af", strings.Join(af, ",")}, a...)
	}
	return a
}

// ---------------- execução ----------------

type deque struct {
	mu    sync.Mutex
	items []*Item // do mais longo para o mais curto
}

func (q *deque) pop(front bool) *Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	if front {
		it := q.items[0]
		q.items = q.items[1:]
		return it
	}
	it := q.items[len(q.items)-1]
	q.items = q.items[:len(q.items)-1]
	return it
}

func (j *Job) waitIfPaused() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	for (j.paused || j.hot) && !j.canceled {
		j.cond.Wait()
	}
	return !j.canceled
}

func (j *Job) applyPause() { // chamado com j.mu travado
	stop := j.paused || j.hot
	for _, p := range j.procs {
		if stop {
			suspendProcess(p)
		} else {
			resumeProcess(p)
		}
	}
	if !stop {
		j.cond.Broadcast()
	}
}

func render(cfg Config, it *Item, dst string, e encoder, threads int, p probe) error {
	part := strings.TrimSuffix(dst, ".mp4") + ".part.mp4"
	bg, ov := assetPaths(cfg, it.Style)
	args := []string{"-y", "-hide_banner", "-loglevel", "warning", "-nostats", "-progress", "pipe:1",
		"-filter_complex_threads", strconv.Itoa(threads),
		"-stream_loop", "-1", "-i", bg}
	if e.gpu && e.hwaccel != "" {
		args = append(args, "-hwaccel", e.hwaccel)
	}
	args = append(args, "-threads", strconv.Itoa(threads))
	if cfg.MaxDur > 0 {
		args = append(args, "-t", strconv.Itoa(cfg.MaxDur))
	}
	args = append(args, "-i", it.Src, "-i", ov, "-filter_complex", buildFilter(cfg, e.pixfmt), "-map", "[v]")
	if p.audio != "" {
		args = append(args, "-map", "1:a:0")
	}
	args = append(args, encArgs(e, cfg.Quality, threads)...)
	args = append(args, audioArgs(cfg, p)...)
	args = append(args, "-r", "30", "-movflags", "+faststart", "-f", "mp4", part)

	cmd := exec.Command(job.ffmpeg, args...)
	hideWindow(cmd)
	lowPriority(cmd, cfg.Priority == "baixa")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	job.mu.Lock()
	job.procs[cmd.Process.Pid] = cmd.Process
	if job.paused || job.hot {
		suspendProcess(cmd.Process)
	}
	job.mu.Unlock()
	dur := it.Dur
	if cfg.MaxDur > 0 && (dur == 0 || float64(cfg.MaxDur) < dur) {
		dur = float64(cfg.MaxDur)
	}
	sc := bufio.NewScanner(out)
	var fps, speed string
	lastMark := -1
	label := "CPU"
	if e.gpu {
		label = "GPU"
	}
	logf("   [%s] %s: encoder %s, %d thread(s)", label, it.Name, e.name, threads)
	for sc.Scan() {
		l := sc.Text()
		if strings.HasPrefix(l, "fps=") {
			fps = strings.TrimPrefix(l, "fps=")
		} else if strings.HasPrefix(l, "speed=") {
			speed = strings.TrimSpace(strings.TrimPrefix(l, "speed="))
		}
		if strings.HasPrefix(l, "out_time_us=") && dur > 0 {
			us, _ := strconv.ParseFloat(strings.TrimPrefix(l, "out_time_us="), 64)
			pct := us / 1e6 / dur * 100
			if pct > 99 {
				pct = 99
			}
			if pct < 0 {
				pct = 0
			}
			job.mu.Lock()
			it.Pct = pct
			job.mu.Unlock()
			if mark := int(pct) / 10 * 10; mark > lastMark {
				lastMark = mark
				logf("   [%s] %s: %d%% | %s fps | velocidade %s", label, it.Name, mark, fps, speed)
			}
		}
	}
	io.Copy(io.Discard, out)
	werr := cmd.Wait()
	job.mu.Lock()
	delete(job.procs, cmd.Process.Pid)
	canceled := job.canceled
	job.mu.Unlock()
	if werr != nil || canceled {
		os.Remove(part)
		if canceled {
			return fmt.Errorf("cancelado")
		}
		msg := strings.TrimSpace(stderr.String())
		logf("ERRO ffmpeg (%s, %s): %s", e.name, it.Name, msg)
		if len(msg) > 240 {
			msg = msg[len(msg)-240:]
		}
		return fmt.Errorf("%s", msg)
	}
	return os.Rename(part, dst)
}

func pickStyles(n int, mode string) []string {
	out := make([]string, n)
	for i := range out {
		switch {
		case mode != "aleatorio":
			out[i] = mode
		case i%2 == 0:
			out[i] = "hud"
		default:
			out[i] = "forja"
		}
	}
	if mode == "aleatorio" {
		rand.Shuffle(n, func(i, k int) { out[i], out[k] = out[k], out[i] })
	}
	return out
}

func startJob(cfg Config) error {
	job.mu.Lock()
	if job.running {
		job.mu.Unlock()
		return fmt.Errorf("já existe um processamento em andamento")
	}
	job.mu.Unlock()
	if st, err := os.Stat(cfg.Folder); err != nil || !st.IsDir() {
		return fmt.Errorf("pasta de vídeos não encontrada: %s", cfg.Folder)
	}
	out := cfg.outDir()
	if err := os.MkdirAll(out, 0o755); err != nil {
		return fmt.Errorf("não consegui criar a pasta de saída: %v", err)
	}
	if logFile != nil {
		logFile.Close()
	}
	logFile, _ = os.OpenFile(filepath.Join(out, "zaptaku_log.txt"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	scanDone(out, cfg.Quality, true) // limpa sobras .part
	items, err := buildItems(cfg)
	if err != nil {
		return err
	}
	var todo []*Item
	for _, it := range items {
		if it.Status == "fila" {
			todo = append(todo, it)
		}
	}
	styles := pickStyles(len(todo), cfg.Mode)
	for i, it := range todo {
		it.Style = styles[i]
	}
	job.mu.Lock()
	job.cfg, job.items, job.running, job.paused, job.canceled = cfg, items, true, false, false
	job.startedAt, job.doneSecs, job.encoder, job.plan = time.Now(), 0, "", "Preparando..."
	job.mu.Unlock()
	saveConfig(cfg)
	logf("===== início: %s | %s | template %s | motor %s =====", cfg.Folder, cfg.Quality, cfg.Mode, cfg.Engine)
	go runJob(cfg, todo)
	return nil
}

func finishJob(msg string) {
	job.mu.Lock()
	job.running = false
	job.finished = time.Now()
	job.plan = msg
	cfg := job.cfg
	canceled := job.canceled
	job.mu.Unlock()
	logf("%s", msg)
	if canceled {
		return
	}
	switch cfg.OnFinish {
	case "abrir":
		openFolder(cfg.outDir())
	case "desligar":
		logf("desligando o PC em 60 segundos (cancele com: shutdown /a)")
		shutdownPC()
	}
}

func runJob(cfg Config, todo []*Item) {
	if len(todo) == 0 {
		finishJob("Tudo já estava pronto nessa qualidade.")
		return
	}
	for _, it := range todo {
		p := probeMedia(job.ffmpeg, it.Src)
		it.Dur = p.dur
	}
	need := map[string]bool{}
	for _, it := range todo {
		need[it.Style] = true
	}
	var styles []string
	for s := range need {
		styles = append(styles, s)
	}
	job.mu.Lock()
	job.plan = "Preparando templates..."
	job.mu.Unlock()
	if err := prepareAssets(cfg, styles); err != nil {
		finishJob("Erro ao preparar templates: " + err.Error())
		return
	}

	gpuEnc, hasGPU := encoder{}, false
	if cfg.Engine != "cpu" {
		job.mu.Lock()
		job.plan = "Testando a placa de vídeo..."
		job.mu.Unlock()
		gpuEnc, hasGPU = detectGPU(job.ffmpeg, cfg.Quality)
	}
	cores := runtime.NumCPU()
	gpuJobs, cpuJobs := 0, 2
	if hasGPU {
		gpuJobs, cpuJobs = 2, 1
	}
	if cores < 8 {
		if hasGPU {
			gpuJobs, cpuJobs = 1, 0
		} else {
			cpuJobs = 1
		}
	}
	if cfg.Engine == "gpu" {
		cpuJobs = 0
		if !hasGPU {
			finishJob("Nenhuma GPU disponível. Escolha o motor Automático ou Só CPU. Detalhes em zaptaku_log.txt.")
			return
		}
	}
	if cfg.GPUJobs > 0 && hasGPU {
		gpuJobs = cfg.GPUJobs
	}
	if cfg.CPUJobs >= 0 && cfg.Engine != "gpu" {
		cpuJobs = cfg.CPUJobs
	}
	if !hasGPU {
		gpuJobs = 0
	}
	if gpuJobs+cpuJobs == 0 {
		cpuJobs = 1
	}
	for gpuJobs+cpuJobs > len(todo) {
		if cpuJobs > 0 && gpuJobs > 0 {
			cpuJobs--
		} else if gpuJobs > 1 {
			gpuJobs--
		} else {
			cpuJobs--
		}
	}
	threads := cores / (gpuJobs + cpuJobs)
	if threads < 2 {
		threads = 2
	}
	encName := "CPU (x264)"
	if hasGPU {
		encName = gpuEnc.name + " + CPU (x264)"
		if cpuJobs == 0 {
			encName = gpuEnc.name
		}
	}
	job.mu.Lock()
	job.encoder = encName
	job.plan = fmt.Sprintf("%d na GPU + %d na CPU ao mesmo tempo, %d threads cada", gpuJobs, cpuJobs, threads)
	if !hasGPU {
		job.plan = fmt.Sprintf("%d na CPU ao mesmo tempo, %d threads cada", cpuJobs, threads)
	}
	job.mu.Unlock()
	logf("plano: %s", job.plan)

	sort.SliceStable(todo, func(a, b int) bool { return todo[a].Dur > todo[b].Dur })
	q := &deque{items: todo}
	var wg sync.WaitGroup
	worker := func(enc encoder, label string, front bool) {
		defer wg.Done()
		for job.waitIfPaused() {
			it := q.pop(front)
			if it == nil {
				return
			}
			suffix := "_zaptaku_" + it.Style
			if cfg.Quality == "4k" {
				suffix += "_4k"
			}
			dst := filepath.Join(cfg.outDir(), it.Base+suffix+".mp4")
			p := probeMedia(job.ffmpeg, it.Src)
			job.mu.Lock()
			it.Status, it.Worker, it.Pct, it.started = "rodando", label, 0, time.Now()
			job.mu.Unlock()
			logf("▶ [%s] %s (%s)", label, it.Name, it.Style)
			err := render(cfg, it, dst, enc, threads, p)
			if err != nil && enc.gpu && err.Error() != "cancelado" {
				logf("GPU falhou em %s, refazendo pela CPU", it.Name)
				job.mu.Lock()
				it.Worker, it.Pct = "CPU", 0
				job.mu.Unlock()
				err = render(cfg, it, dst, cpuEncoder, threads, p)
			}
			job.mu.Lock()
			it.Secs = time.Since(it.started).Seconds()
			switch {
			case err == nil:
				it.Status, it.Pct, it.Out = "ok", 100, filepath.Base(dst)
				job.doneSecs += it.Dur
			case err.Error() == "cancelado":
				it.Status = "cancelado"
			default:
				it.Status, it.Err = "erro", err.Error()
			}
			job.mu.Unlock()
			if err == nil {
				logf("✔ [%s] %s em %.0fs", label, filepath.Base(dst), it.Secs)
			}
		}
	}
	for i := 0; i < gpuJobs; i++ {
		wg.Add(1)
		go worker(gpuEnc, "GPU", true)
	}
	for i := 0; i < cpuJobs; i++ {
		wg.Add(1)
		go worker(cpuEncoder, "CPU", gpuJobs == 0)
	}
	wg.Wait()
	job.mu.Lock()
	ok, bad := 0, 0
	for _, it := range job.items {
		if it.Status == "fila" && job.canceled {
			it.Status = "cancelado"
		}
		if it.Status == "ok" {
			ok++
		}
		if it.Status == "erro" {
			bad++
		}
	}
	canceled := job.canceled
	el := time.Since(job.startedAt).Round(time.Second)
	job.mu.Unlock()
	if canceled {
		finishJob(fmt.Sprintf("Cancelado. %d vídeo(s) ficaram prontos; rode de novo para continuar de onde parou.", ok))
		return
	}
	msg := fmt.Sprintf("Concluído em %s: %d editado(s)", el, ok)
	if bad > 0 {
		msg += fmt.Sprintf(", %d com erro (rode de novo para tentar só esses)", bad)
	}
	finishJob(msg + ".")
}

func pauseJob(p bool) {
	job.mu.Lock()
	job.paused = p
	job.applyPause()
	job.mu.Unlock()
	if p {
		logf("pausado pelo usuário")
	} else {
		logf("retomado pelo usuário")
	}
}

func cancelJob() {
	job.mu.Lock()
	job.canceled, job.paused, job.hot = true, false, false
	for _, p := range job.procs {
		resumeProcess(p)
		p.Kill()
	}
	job.cond.Broadcast()
	job.mu.Unlock()
	logf("cancelado pelo usuário")
}

// trava de temperatura: congela as edições acima do limite e retoma ao esfriar.
func thermalLoop() {
	for range time.Tick(2 * time.Second) {
		t := telemetry.snapshot()
		job.mu.Lock()
		cfg := job.cfg
		if job.running {
			hot := ""
			if t.GPU.Temp >= cfg.GPUMax && cfg.GPUMax > 0 {
				hot = fmt.Sprintf("GPU %d°C", t.GPU.Temp)
			} else if t.CPU.Temp >= cfg.CPUMax && cfg.CPUMax > 0 {
				hot = fmt.Sprintf("CPU %d°C", t.CPU.Temp)
			}
			cool := (t.GPU.Temp < 0 || t.GPU.Temp <= cfg.GPUResume) && (t.CPU.Temp < 0 || t.CPU.Temp <= cfg.CPUResume)
			if !job.hot && hot != "" {
				job.hot, job.hotReason = true, hot
				job.applyPause()
				job.mu.Unlock()
				logf("🌡 temperatura alta (%s): pausando até esfriar", hot)
				continue
			}
			if job.hot && cool {
				job.hot, job.hotReason = false, ""
				job.applyPause()
				job.mu.Unlock()
				logf("✅ temperatura normalizada: retomando")
				continue
			}
		}
		job.mu.Unlock()
	}
}

// ---------------- snapshot para a interface ----------------

type Snapshot struct {
	Running   bool      `json:"running"`
	Paused    bool      `json:"paused"`
	Hot       bool      `json:"hot"`
	HotReason string    `json:"hotReason"`
	Encoder   string    `json:"encoder"`
	Plan      string    `json:"plan"`
	Items     []Item    `json:"items"`
	Elapsed   float64   `json:"elapsed"`
	ETA       float64   `json:"eta"` // segundos, -1 = calculando
	Logs      []string  `json:"logs"`
	Tele      Telemetry `json:"tele"`
	Config    Config    `json:"config"`
}

func snapshot() Snapshot {
	job.mu.Lock()
	defer job.mu.Unlock()
	s := Snapshot{Running: job.running, Paused: job.paused, Hot: job.hot, HotReason: job.hotReason,
		Encoder: job.encoder, Plan: job.plan, Config: job.cfg, ETA: -1}
	var remain, progressed float64
	for _, it := range job.items {
		c := *it
		if it.Status == "rodando" {
			c.Secs = time.Since(it.started).Seconds()
			progressed += it.Dur * it.Pct / 100
			remain += it.Dur * (1 - it.Pct/100)
		}
		if it.Status == "fila" {
			remain += it.Dur
		}
		s.Items = append(s.Items, c)
	}
	if job.running {
		s.Elapsed = time.Since(job.startedAt).Seconds()
		if done := job.doneSecs + progressed; done > 1 && s.Elapsed > 5 {
			s.ETA = remain / (done / s.Elapsed)
		}
	} else if !job.finished.IsZero() {
		s.Elapsed = job.finished.Sub(job.startedAt).Seconds()
	}
	n := len(job.logs)
	if n > 150 {
		s.Logs = append([]string(nil), job.logs[n-150:]...)
	} else {
		s.Logs = append([]string(nil), job.logs...)
	}
	s.Tele = telemetry.snapshot()
	return s
}
