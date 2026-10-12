// Zaptaku Editor: aplica os templates do @zaptaku em todos os vídeos de uma pasta.
// - Retoma de onde parou: pula vídeos que já estão prontos na pasta de editados.
// - Usa GPU (NVENC / Quick Sync / AMF) e CPU (x264) ao mesmo tempo.
// - Trava de temperatura: pausa tudo se GPU ou CPU esquentarem demais.
package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
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

//go:embed assets/loop_hud.mp4 assets/loop_forja.mp4 assets/overlay_hud.png assets/overlay_forja.png
var assets embed.FS

var videoExt = map[string]bool{".mp4": true, ".mov": true, ".mkv": true, ".webm": true, ".avi": true, ".m4v": true}

const outDirName = "zaptaku_editados"

var in = bufio.NewReader(os.Stdin)

func ask(prompt string) string {
	fmt.Print(prompt)
	s, _ := in.ReadString('\n')
	return strings.TrimSpace(s)
}

func pause() { ask("\nAperte ENTER para fechar...") }

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v >= 0 {
		return v
	}
	return def
}

// ---------------- log ----------------

var logMu sync.Mutex
var logFile *os.File

func logf(format string, a ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		fmt.Fprintf(logFile, time.Now().Format("15:04:05 ")+format+"\n", a...)
	}
}

// ---------------- arquivos ----------------

func extract(dir string) error {
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

// Lê a pasta de editados e devolve o conjunto de vídeos (nome base) já prontos
// para a qualidade escolhida. Remove sobras de execuções interrompidas (.part.mp4).
func scanDone(out, quality string) map[string]bool {
	done := map[string]bool{}
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
			os.Remove(filepath.Join(out, n))
			logf("removida sobra incompleta: %s", n)
			continue
		}
		if !strings.HasSuffix(strings.ToLower(n), ".mp4") {
			continue
		}
		if info, err := e.Info(); err != nil || info.Size() < 10_000 {
			continue
		}
		stem := strings.TrimSuffix(n, filepath.Ext(n))
		is4k := strings.HasSuffix(stem, "_4k")
		if (quality == "4k") != is4k {
			continue
		}
		for _, tag := range []string{"_zaptaku_", "_nakama_"} {
			if i := strings.LastIndex(stem, tag); i > 0 {
				done[stem[:i]] = true
				break
			}
		}
	}
	return done
}

// no modo aleatório alterna hud/forja em ordem embaralhada, sem sair tudo igual.
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
		rand.Shuffle(n, func(i, j int) { out[i], out[j] = out[j], out[i] })
	}
	return out
}

// ---------------- codificadores ----------------

type encoder struct {
	name   string
	codec  string
	gpu    bool
	pixfmt string
}

var cpuEncoder = encoder{"CPU (x264)", "libx264", false, "yuv420p"}

var gpuEncoders = []encoder{
	{"NVIDIA (NVENC)", "h264_nvenc", true, "nv12"},
	{"Intel (Quick Sync)", "h264_qsv", true, "nv12"},
	{"AMD (AMF)", "h264_amf", true, "nv12"},
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
			"-rc", "vbr", "-cq", cq, "-b:v", "0", "-maxrate", maxr, "-bufsize", buf, "-spatial-aq", "1"}
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

// testa o codificador de verdade, na resolução final e com as mesmas opções.
func encoderTest(ffmpeg string, e encoder, quality string) error {
	size := "1080x1920"
	if quality == "4k" {
		size = "2160x3840"
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=black:s=" + size + ":d=0.5:r=30",
		"-vf", "format=" + e.pixfmt}
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
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func detectGPU(ffmpeg, quality string) (encoder, bool) {
	for _, e := range gpuEncoders {
		err := encoderTest(ffmpeg, e, quality)
		if err == nil {
			logf("GPU OK: %s", e.name)
			return e, true
		}
		first := strings.SplitN(err.Error(), "\n", 2)[0]
		logf("GPU %s indisponível: %v", e.name, err)
		fmt.Printf("  - %s: não disponível (%s)\n", e.name, truncate(first, 90))
	}
	return encoder{}, false
}

// ---------------- render ----------------

var reDur = regexp.MustCompile(`Duration: (\d+):(\d+):(\d+(?:\.\d+)?)`)

func probeDuration(ffmpeg, src string) float64 {
	cmd := exec.Command(ffmpeg, "-hide_banner", "-i", src)
	hideWindow(cmd)
	var buf bytes.Buffer
	cmd.Stderr = &buf
	_ = cmd.Run() // sai com erro (sem arquivo de saída), mas imprime a duração
	m := reDur.FindStringSubmatch(buf.String())
	if m == nil {
		return 0
	}
	h, _ := strconv.ParseFloat(m[1], 64)
	mi, _ := strconv.ParseFloat(m[2], 64)
	s, _ := strconv.ParseFloat(m[3], 64)
	return h*3600 + mi*60 + s
}

func buildFilter(quality, pixfmt string) string {
	if quality == "4k" {
		return "[1:v]hqdn3d=1.5:1.5:6:6," +
			"scale=2160:1216:force_original_aspect_ratio=decrease:flags=lanczos," +
			"pad=2160:1216:(ow-iw)/2:(oh-ih)/2:color=0x060610,setsar=1," +
			"unsharp=5:5:0.6:5:5:0.0,eq=contrast=1.03:saturation=1.08,fps=30[clip];" +
			"[0:v]scale=2160:3840:flags=bicubic,setsar=1[bg];" +
			"[bg][clip]overlay=0:1312:shortest=1[b];" +
			"[2:v]scale=2160:3840:flags=lanczos[ov];" +
			"[b][ov]overlay=0:0,format=" + pixfmt + "[v]"
	}
	return "[1:v]scale=1080:608:force_original_aspect_ratio=decrease," +
		"pad=1080:608:(ow-iw)/2:(oh-ih)/2:color=0x060610,setsar=1,fps=30[clip];" +
		"[0:v][clip]overlay=0:656:shortest=1[b];" +
		"[b][2:v]overlay=0:0,format=" + pixfmt + "[v]"
}

type job struct {
	idx   int
	src   string
	base  string
	style string
	dur   float64
}

// fila de duas pontas: GPU pega os vídeos mais longos, CPU pega os mais curtos.
type deque struct {
	mu    sync.Mutex
	items []job // ordenado do mais longo para o mais curto
}

func (q *deque) pop(front bool) (job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return job{}, false
	}
	if front {
		j := q.items[0]
		q.items = q.items[1:]
		return j, true
	}
	j := q.items[len(q.items)-1]
	q.items = q.items[:len(q.items)-1]
	return j, true
}

// ---------------- progresso + temperatura ----------------

type state struct {
	mu      sync.Mutex
	pct     map[int]float64
	name    map[int]string
	who     map[int]string
	done    int
	all     int
	gpuT    int // -1 = sem leitura
	cpuT    int
	paused  bool
	reason  string
	procs   map[int]*os.Process
	resumed *sync.Cond
}

func newState(all int) *state {
	s := &state{pct: map[int]float64{}, name: map[int]string{}, who: map[int]string{}, all: all, gpuT: -1, cpuT: -1, procs: map[int]*os.Process{}}
	s.resumed = sync.NewCond(&s.mu)
	return s
}

func (s *state) set(i int, v float64) { s.mu.Lock(); s.pct[i] = v; s.mu.Unlock() }

func (s *state) waitIfPaused() {
	s.mu.Lock()
	for s.paused {
		s.resumed.Wait()
	}
	s.mu.Unlock()
}

func tstr(t int) string {
	if t < 0 {
		return "n/d"
	}
	return fmt.Sprintf("%d°C", t)
}

func (s *state) line() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]int, 0, len(s.pct))
	for k := range s.pct {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	parts := []string{fmt.Sprintf("%d/%d prontos", s.done, s.all), "GPU " + tstr(s.gpuT) + " CPU " + tstr(s.cpuT)}
	if s.paused {
		parts = append(parts, "⏸ PAUSADO: "+s.reason+" — esfriando")
	}
	for _, k := range keys {
		n := []rune(s.name[k])
		if len(n) > 12 {
			n = append(n[:11], '…')
		}
		parts = append(parts, fmt.Sprintf("[%s] %s %3.0f%%", s.who[k], string(n), s.pct[k]))
	}
	return strings.Join(parts, " | ")
}

var printMu sync.Mutex

func say(format string, a ...any) {
	printMu.Lock()
	fmt.Printf("\r%-140s\r", "")
	fmt.Printf(format+"\n", a...)
	printMu.Unlock()
	logf(format, a...)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// Trava de temperatura: pausa (congela) todas as codificações acima do limite
// e só retoma quando esfriar abaixo do limite de retomada.
func thermalGuard(s *state, stop <-chan struct{}) {
	gpuMax := envInt("ZAPTAKU_GPU_MAX", 83)
	gpuOk := envInt("ZAPTAKU_GPU_RETOMA", 75)
	cpuMax := envInt("ZAPTAKU_CPU_MAX", 90)
	cpuOk := envInt("ZAPTAKU_CPU_RETOMA", 80)
	t := time.NewTicker(4 * time.Second)
	defer t.Stop()
	for {
		g, c := gpuTemp(), cpuTemp()
		s.mu.Lock()
		s.gpuT, s.cpuT = g, c
		hot := ""
		if g >= gpuMax {
			hot = fmt.Sprintf("GPU %d°C", g)
		} else if c >= cpuMax {
			hot = fmt.Sprintf("CPU %d°C", c)
		}
		cool := (g < 0 || g <= gpuOk) && (c < 0 || c <= cpuOk)
		switch {
		case !s.paused && hot != "":
			s.paused, s.reason = true, hot
			for _, p := range s.procs {
				suspendProcess(p)
			}
			s.mu.Unlock()
			say("🌡 Temperatura alta (%s): pausando tudo até esfriar...", hot)
			s.mu.Lock()
		case s.paused && cool:
			s.paused = false
			for _, p := range s.procs {
				resumeProcess(p)
			}
			s.resumed.Broadcast()
			s.mu.Unlock()
			say("✅ Temperatura normalizada (GPU %s, CPU %s): retomando.", tstr(g), tstr(c))
			s.mu.Lock()
		}
		s.mu.Unlock()
		select {
		case <-stop:
			return
		case <-t.C:
		}
	}
}

func render(ffmpeg, tmp string, j job, dst, quality string, e encoder, threads int, st *state) error {
	part := strings.TrimSuffix(dst, ".mp4") + ".part.mp4"
	args := []string{"-y", "-hide_banner", "-loglevel", "error", "-nostats", "-progress", "pipe:1",
		"-filter_complex_threads", strconv.Itoa(threads),
		"-stream_loop", "-1", "-i", filepath.Join(tmp, "loop_"+j.style+".mp4")}
	if e.gpu {
		args = append(args, "-hwaccel", "auto") // decodificação pela GPU quando possível
	}
	args = append(args, "-i", j.src, "-i", filepath.Join(tmp, "overlay_"+j.style+".png"),
		"-filter_complex", buildFilter(quality, e.pixfmt),
		"-map", "[v]", "-map", "1:a?")
	args = append(args, encArgs(e, quality, threads)...)
	args = append(args, "-r", "30", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", "-f", "mp4", part)

	cmd := exec.Command(ffmpeg, args...)
	hideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	st.mu.Lock()
	st.procs[cmd.Process.Pid] = cmd.Process
	if st.paused { // começou durante uma pausa
		suspendProcess(cmd.Process)
	}
	st.mu.Unlock()
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		l := sc.Text()
		if strings.HasPrefix(l, "out_time_ms=") && j.dur > 0 {
			us, _ := strconv.ParseFloat(strings.TrimPrefix(l, "out_time_ms="), 64)
			p := us / 1e6 / j.dur * 100
			if p > 99 {
				p = 99
			}
			if p < 0 {
				p = 0
			}
			st.set(j.idx, p)
		}
	}
	io.Copy(io.Discard, out)
	werr := cmd.Wait()
	st.mu.Lock()
	delete(st.procs, cmd.Process.Pid)
	st.mu.Unlock()
	if werr != nil {
		os.Remove(part)
		msg := strings.TrimSpace(stderr.String())
		logf("ERRO ffmpeg (%s, %s): %s", e.name, filepath.Base(j.src), msg)
		if len(msg) > 300 {
			msg = msg[len(msg)-300:]
		}
		return fmt.Errorf("%v: %s", werr, msg)
	}
	return os.Rename(part, dst) // só vira "pronto" quando terminou inteiro
}

func main() {
	setupConsole()
	defer runCleanups()
	fmt.Println("==============================================")
	fmt.Println("   ZAPTAKU EDITOR  -  templates @zaptaku")
	fmt.Println("==============================================")

	folder := ""
	if len(os.Args) > 1 {
		folder = os.Args[1]
	} else {
		fmt.Println("\nEscolha a pasta com os vídeos na janela que abriu...")
		folder = pickFolder()
	}
	if folder == "" {
		folder = ask("Cole o caminho da pasta e aperte ENTER: ")
	}
	folder = strings.Trim(folder, "\"")
	if st, err := os.Stat(folder); err != nil || !st.IsDir() {
		fmt.Println("Pasta inválida:", folder)
		pause()
		return
	}
	vids, err := listVideos(folder)
	if err != nil || len(vids) == 0 {
		fmt.Println("Nenhum vídeo (.mp4, .mov, .mkv, .webm, .avi, .m4v) encontrado em:", folder)
		pause()
		return
	}

	fmt.Println("\nQual template?")
	fmt.Println("  1 - Aleatório, alternando Tela de RPG e Forja (padrão)")
	fmt.Println("  2 - Só Tela de RPG")
	fmt.Println("  3 - Só Forja")
	mode := "aleatorio"
	switch ask("Escolha [1/2/3] e aperte ENTER: ") {
	case "2":
		mode = "hud"
	case "3":
		mode = "forja"
	}
	fmt.Println("\nQual qualidade?")
	fmt.Println("  1 - 4K otimizado 2160x3840 (padrão)")
	fmt.Println("  2 - 1080p 1080x1920")
	quality := "4k"
	if ask("Escolha [1/2] e aperte ENTER: ") == "2" {
		quality = "1080"
	}

	out := filepath.Join(folder, outDirName)
	os.MkdirAll(out, 0o755)
	logFile, _ = os.OpenFile(filepath.Join(out, "zaptaku_log.txt"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if logFile != nil {
		defer logFile.Close()
	}
	logf("===== nova execução: %s | qualidade %s | modo %s =====", folder, quality, mode)

	// ---- retomar: o que já está pronto ----
	done := scanDone(out, quality)
	var todo []string
	for _, v := range vids {
		if !done[strings.TrimSuffix(filepath.Base(v), filepath.Ext(v))] {
			todo = append(todo, v)
		}
	}
	fmt.Printf("\nPasta: %s\n%d vídeo(s) no total | %d já pronto(s) em %s | %d para fazer.\n",
		folder, len(vids), len(vids)-len(todo), outDirName, len(todo))
	if len(todo) == 0 {
		fmt.Println("Tudo já foi editado nessa qualidade. Nada a fazer.")
		openFolder(out)
		pause()
		return
	}

	tmp, err := os.MkdirTemp("", "zaptaku-editor-")
	if err != nil {
		fmt.Println("Erro ao criar pasta temporária:", err)
		pause()
		return
	}
	defer os.RemoveAll(tmp)
	if err := extract(tmp); err != nil {
		fmt.Println("Erro ao preparar os templates:", err)
		pause()
		return
	}
	fmt.Println("\nPreparando o motor de vídeo (só demora na primeira vez)...")
	ffmpeg, err := prepareFFmpeg(tmp)
	if err != nil {
		fmt.Println("Erro ao preparar o ffmpeg:", err)
		pause()
		return
	}

	// ---- GPU + CPU ----
	fmt.Println("\nDetectando placa de vídeo...")
	if info := gpuInfo(); info != "" {
		fmt.Println("  Placa encontrada:", info)
		logf("nvidia-smi: %s", info)
	}
	gpuEnc, hasGPU := detectGPU(ffmpeg, quality)
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
	gpuJobs = envInt("ZAPTAKU_GPU_JOBS", gpuJobs)
	cpuJobs = envInt("ZAPTAKU_CPU_JOBS", cpuJobs)
	if !hasGPU {
		gpuJobs = 0
	}
	if gpuJobs+cpuJobs == 0 {
		cpuJobs = 1
	}
	if total := gpuJobs + cpuJobs; total > len(todo) { // não abre mais trabalhadores que vídeos
		for gpuJobs+cpuJobs > len(todo) {
			if cpuJobs > 0 && (gpuJobs > 0) {
				cpuJobs--
			} else if gpuJobs > 1 {
				gpuJobs--
			} else {
				cpuJobs--
			}
		}
	}
	threads := cores / (gpuJobs + cpuJobs)
	if threads < 2 {
		threads = 2
	}
	if hasGPU {
		fmt.Printf("Usando: %s (%d vídeo(s) por vez) + CPU x264 (%d vídeo(s) por vez) | %d threads cada\n", gpuEnc.name, gpuJobs, cpuJobs, threads)
	} else {
		fmt.Printf("GPU não disponível — usando só a CPU (%d vídeo(s) por vez, %d threads cada).\n", cpuJobs, threads)
		fmt.Println("Detalhes do motivo foram salvos em", filepath.Join(outDirName, "zaptaku_log.txt"))
	}
	fmt.Printf("Trava de temperatura: pausa com GPU ≥ %d°C ou CPU ≥ %d°C.\n\n",
		envInt("ZAPTAKU_GPU_MAX", 83), envInt("ZAPTAKU_CPU_MAX", 90))

	styles := pickStyles(len(todo), mode)
	names := map[string]string{"hud": "Tela de RPG", "forja": "Forja"}
	var jobs []job
	for i, v := range todo {
		base := strings.TrimSuffix(filepath.Base(v), filepath.Ext(v))
		jobs = append(jobs, job{idx: i, src: v, base: base, style: styles[i], dur: probeDuration(ffmpeg, v)})
	}
	sort.SliceStable(jobs, func(a, b int) bool { return jobs[a].dur > jobs[b].dur })
	q := &deque{items: jobs}

	st := newState(len(jobs))
	stop := make(chan struct{})
	go thermalGuard(st, stop)
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				printMu.Lock()
				fmt.Printf("\r%-140s", truncate(st.line(), 140))
				printMu.Unlock()
			}
		}
	}()

	var wg sync.WaitGroup
	var okN, failN, cpuRetry int
	var cntMu sync.Mutex
	start := time.Now()

	worker := func(enc encoder, label string, front bool) {
		defer wg.Done()
		for {
			st.waitIfPaused()
			j, ok := q.pop(front)
			if !ok {
				return
			}
			suffix := "_zaptaku_" + j.style
			if quality == "4k" {
				suffix += "_4k"
			}
			dst := filepath.Join(out, j.base+suffix+".mp4")
			name := filepath.Base(j.src)
			st.mu.Lock()
			st.pct[j.idx], st.name[j.idx], st.who[j.idx] = 0, name, label
			st.mu.Unlock()
			say("▶ [%s] começando: %s  (%s)", label, name, names[j.style])
			t0 := time.Now()
			err := render(ffmpeg, tmp, j, dst, quality, enc, threads, st)
			if err != nil && enc.gpu {
				say("  ⚠ GPU falhou em %s, refazendo pela CPU...", name)
				st.set(j.idx, 0)
				st.mu.Lock()
				st.who[j.idx] = "CPU"
				st.mu.Unlock()
				err = render(ffmpeg, tmp, j, dst, quality, cpuEncoder, threads, st)
				cntMu.Lock()
				cpuRetry++
				cntMu.Unlock()
			}
			st.mu.Lock()
			delete(st.pct, j.idx)
			st.done++
			st.mu.Unlock()
			cntMu.Lock()
			if err != nil {
				failN++
				say("✖ ERRO em %s: %v", name, err)
			} else {
				okN++
				say("✔ [%s] pronto em %s: %s", label, time.Since(t0).Round(time.Second), filepath.Base(dst))
			}
			cntMu.Unlock()
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
	close(stop)

	printMu.Lock()
	fmt.Printf("\r%-140s\r", "")
	printMu.Unlock()
	fmt.Println("\n==============================================")
	fmt.Printf("Concluído em %s: %d editado(s) agora, %d já estavam prontos, %d com erro.\n",
		time.Since(start).Round(time.Second), okN, len(vids)-len(todo), failN)
	if cpuRetry > 0 {
		fmt.Printf("(%d vídeo(s) precisaram ser refeitos pela CPU — veja zaptaku_log.txt)\n", cpuRetry)
	}
	if failN > 0 {
		fmt.Println("Rode de novo para tentar só os que deram erro: os prontos serão pulados.")
	}
	fmt.Println("Vídeos salvos em:", out)
	logf("fim: %d ok, %d erro, %d refeitos na CPU", okN, failN, cpuRetry)
	runCleanups()
	openFolder(out)
	pause()
}
