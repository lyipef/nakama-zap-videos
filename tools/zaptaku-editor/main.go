// Zaptaku Editor: aplica os templates do @zaptaku em todos os vídeos de uma pasta.
// Usa a GPU (NVENC / QSV / AMF) quando disponível e processa vários vídeos em paralelo.
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

var encoders = []encoder{
	{"NVIDIA (NVENC)", "h264_nvenc", true, "nv12"},
	{"Intel (Quick Sync)", "h264_qsv", true, "nv12"},
	{"AMD (AMF)", "h264_amf", true, "nv12"},
	{"CPU (x264)", "libx264", false, "yuv420p"},
}

func encArgs(e encoder, quality string, threads int) []string {
	hq := quality == "4k"
	switch e.codec {
	case "h264_nvenc":
		cq, maxr, buf := "21", "20M", "40M"
		if hq {
			cq, maxr, buf = "19", "80M", "160M"
		}
		return []string{"-c:v", "h264_nvenc", "-preset", "slow", "-profile:v", "high", "-rc", "vbr_hq",
			"-cq", cq, "-b:v", "0", "-maxrate", maxr, "-bufsize", buf, "-spatial-aq", "1"}
	case "h264_qsv":
		q := "22"
		if hq {
			q = "20"
		}
		return []string{"-c:v", "h264_qsv", "-preset", "medium", "-global_quality", q, "-look_ahead", "0"}
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

// testa o codificador de verdade, na resolução final, com as mesmas opções.
func encoderWorks(ffmpeg string, e encoder, quality string) bool {
	size := "1080x1920"
	if quality == "4k" {
		size = "2160x3840"
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=black:s=" + size + ":d=0.3:r=30",
		"-vf", "format=" + e.pixfmt}
	args = append(args, encArgs(e, quality, 2)...)
	args = append(args, "-f", "null", "-")
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	hideWindow(cmd)
	return cmd.Run() == nil
}

func detectEncoder(ffmpeg, quality string) encoder {
	for _, e := range encoders[:3] {
		if encoderWorks(ffmpeg, e, quality) {
			return e
		}
	}
	return encoders[3]
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
	dst   string
	style string
	dur   float64
}

type progress struct {
	mu   sync.Mutex
	pct  map[int]float64
	name map[int]string
	done int
	all  int
}

func (p *progress) set(i int, v float64) { p.mu.Lock(); p.pct[i] = v; p.mu.Unlock() }

func (p *progress) line() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	keys := make([]int, 0, len(p.pct))
	for k := range p.pct {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	parts := []string{fmt.Sprintf("%d/%d prontos", p.done, p.all)}
	for _, k := range keys {
		n := []rune(p.name[k])
		if len(n) > 16 {
			n = append(n[:15], '…')
		}
		parts = append(parts, fmt.Sprintf("%s %3.0f%%", string(n), p.pct[k]))
	}
	return strings.Join(parts, " | ")
}

var printMu sync.Mutex

func say(format string, a ...any) {
	printMu.Lock()
	fmt.Printf("\r%-118s\r", "")
	fmt.Printf(format+"\n", a...)
	printMu.Unlock()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func render(ffmpeg, tmp string, j job, quality string, e encoder, threads int, prog *progress) error {
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
	args = append(args, "-r", "30", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", j.dst)

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
			prog.set(j.idx, p)
		}
	}
	io.Copy(io.Discard, out)
	if err := cmd.Wait(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 300 {
			msg = msg[len(msg)-300:]
		}
		return fmt.Errorf("%v: %s", err, msg)
	}
	return nil
}

func main() {
	setupConsole()
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
	fmt.Printf("\nPasta: %s\n%d vídeo(s) encontrado(s).\n", folder, len(vids))

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
	ffmpeg, err := prepareFFmpeg(tmp)
	if err != nil {
		fmt.Println("Erro ao preparar o ffmpeg:", err)
		pause()
		return
	}

	fmt.Println("\nDetectando placa de vídeo...")
	enc := detectEncoder(ffmpeg, quality)
	cores := runtime.NumCPU()
	workers := 2
	if enc.gpu {
		workers = 3 // limite seguro de codificações simultâneas na GPU
	}
	if cores < 8 {
		workers = 1
	}
	if v, err := strconv.Atoi(os.Getenv("ZAPTAKU_JOBS")); err == nil && v > 0 {
		workers = v // ajuste manual opcional
	}
	if workers > len(vids) {
		workers = len(vids)
	}
	threads := cores / workers
	if threads < 2 {
		threads = 2
	}
	fmt.Printf("Codificação: %s | %d vídeo(s) em paralelo | %d threads cada (%d núcleos lógicos)\n\n", enc.name, workers, threads, cores)

	out := filepath.Join(folder, outDirName)
	os.MkdirAll(out, 0o755)
	styles := pickStyles(len(vids), mode)
	names := map[string]string{"hud": "Tela de RPG", "forja": "Forja"}

	var jobs []job
	skip := 0
	for i, v := range vids {
		base := strings.TrimSuffix(filepath.Base(v), filepath.Ext(v))
		suffix := "_zaptaku_" + styles[i]
		if quality == "4k" {
			suffix += "_4k"
		}
		dst := filepath.Join(out, base+suffix+".mp4")
		if _, err := os.Stat(dst); err == nil {
			fmt.Printf("  já existe, pulando: %s\n", filepath.Base(dst))
			skip++
			continue
		}
		jobs = append(jobs, job{idx: i, src: v, dst: dst, style: styles[i], dur: probeDuration(ffmpeg, v)})
	}

	prog := &progress{pct: map[int]float64{}, name: map[int]string{}, all: len(jobs)}
	queue := make(chan job)
	var wg sync.WaitGroup
	var okN, failN, cpuRetry int
	var cntMu sync.Mutex
	start := time.Now()

	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				printMu.Lock()
				fmt.Printf("\r%-118s", truncate(prog.line(), 118))
				printMu.Unlock()
			}
		}
	}()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				name := filepath.Base(j.src)
				prog.mu.Lock()
				prog.pct[j.idx] = 0
				prog.name[j.idx] = name
				prog.mu.Unlock()
				say("▶ começando: %s  (%s)", name, names[j.style])
				t0 := time.Now()
				err := render(ffmpeg, tmp, j, quality, enc, threads, prog)
				if err != nil && enc.gpu {
					say("  ⚠ GPU falhou em %s, refazendo pela CPU...", name)
					os.Remove(j.dst)
					prog.set(j.idx, 0)
					err = render(ffmpeg, tmp, j, quality, encoders[3], threads, prog)
					cntMu.Lock()
					cpuRetry++
					cntMu.Unlock()
				}
				prog.mu.Lock()
				delete(prog.pct, j.idx)
				prog.done++
				prog.mu.Unlock()
				cntMu.Lock()
				if err != nil {
					failN++
					os.Remove(j.dst)
					say("✖ ERRO em %s: %v", name, err)
				} else {
					okN++
					say("✔ pronto em %s: %s", time.Since(t0).Round(time.Second), filepath.Base(j.dst))
				}
				cntMu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		queue <- j
	}
	close(queue)
	wg.Wait()
	close(stop)

	printMu.Lock()
	fmt.Printf("\r%-118s\r", "")
	printMu.Unlock()
	fmt.Println("\n==============================================")
	fmt.Printf("Concluído em %s: %d editado(s), %d pulado(s), %d com erro.\n", time.Since(start).Round(time.Second), okN, skip, failN)
	if cpuRetry > 0 {
		fmt.Printf("(%d vídeo(s) precisaram ser refeitos pela CPU)\n", cpuRetry)
	}
	fmt.Println("Vídeos salvos em:", out)
	openFolder(out)
	pause()
}
