// Nakama Editor: aplica os templates do @nakama.zap em todos os vídeos de uma pasta.
package main

import (
	"bufio"
	"embed"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed assets/loop_hud.mp4 assets/loop_forja.mp4 assets/overlay_hud.png assets/overlay_forja.png
var assets embed.FS

var videoExt = map[string]bool{".mp4": true, ".mov": true, ".mkv": true, ".webm": true, ".avi": true, ".m4v": true}

const outDirName = "nakama_editados"

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
		if e.IsDir() {
			continue
		}
		if videoExt[strings.ToLower(filepath.Ext(e.Name()))] {
			vids = append(vids, filepath.Join(folder, e.Name()))
		}
	}
	sort.Strings(vids)
	return vids, nil
}

// estilos: no modo aleatório alterna hud/forja em ordem embaralhada, sem sair tudo igual.
func pickStyles(n int, mode string) []string {
	out := make([]string, n)
	if mode != "aleatorio" {
		for i := range out {
			out[i] = mode
		}
		return out
	}
	for i := range out {
		if i%2 == 0 {
			out[i] = "hud"
		} else {
			out[i] = "forja"
		}
	}
	rand.Shuffle(n, func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func render(ffmpeg, tmp, src, dst, style string) error {
	filter := "[1:v]scale=1080:608:force_original_aspect_ratio=decrease," +
		"pad=1080:608:(ow-iw)/2:(oh-ih)/2:color=0x060610,setsar=1,fps=30[clip];" +
		"[0:v][clip]overlay=0:656:shortest=1[b];" +
		"[b][2:v]overlay=0:0,format=yuv420p[v]"
	args := []string{"-y", "-hide_banner", "-loglevel", "error", "-stats",
		"-stream_loop", "-1", "-i", filepath.Join(tmp, "loop_"+style+".mp4"),
		"-i", src,
		"-i", filepath.Join(tmp, "overlay_"+style+".png"),
		"-filter_complex", filter,
		"-map", "[v]", "-map", "1:a?",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-r", "30",
		"-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", dst}
	cmd := exec.Command(ffmpeg, args...)
	hideWindow(cmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	setupConsole()
	fmt.Println("==============================================")
	fmt.Println("   NAKAMA EDITOR  -  templates @nakama.zap")
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

	tmp, err := os.MkdirTemp("", "nakama-editor-")
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

	out := filepath.Join(folder, outDirName)
	os.MkdirAll(out, 0o755)
	styles := pickStyles(len(vids), mode)
	names := map[string]string{"hud": "Tela de RPG", "forja": "Forja"}

	ok, skip, fail := 0, 0, 0
	start := time.Now()
	for i, v := range vids {
		base := strings.TrimSuffix(filepath.Base(v), filepath.Ext(v))
		dst := filepath.Join(out, base+"_nakama_"+styles[i]+".mp4")
		fmt.Printf("\n[%d/%d] %s  ->  %s\n", i+1, len(vids), filepath.Base(v), names[styles[i]])
		if _, err := os.Stat(dst); err == nil {
			fmt.Println("   já existe, pulando.")
			skip++
			continue
		}
		if err := render(ffmpeg, tmp, v, dst, styles[i]); err != nil {
			fmt.Println("\n   ERRO ao editar este vídeo:", err)
			os.Remove(dst)
			fail++
			continue
		}
		fmt.Println("\n   pronto:", filepath.Base(dst))
		ok++
	}

	fmt.Println("\n==============================================")
	fmt.Printf("Concluído em %s: %d editado(s), %d pulado(s), %d com erro.\n", time.Since(start).Round(time.Second), ok, skip, fail)
	fmt.Println("Vídeos salvos em:", out)
	openFolder(out)
	pause()
}
