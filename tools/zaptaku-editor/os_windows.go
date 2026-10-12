//go:build windows

package main

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

//go:embed assets/ffmpeg.exe
var ffmpegExe []byte

func prepareFFmpeg(tmp string) (string, error) {
	p := filepath.Join(tmp, "ffmpeg.exe")
	return p, os.WriteFile(p, ffmpegExe, 0o755)
}

func setupConsole() {
	k := syscall.NewLazyDLL("kernel32.dll")
	k.NewProc("SetConsoleOutputCP").Call(65001)
	k.NewProc("SetConsoleCP").Call(65001)
}

func hideWindow(cmd *exec.Cmd) {}

func pickFolder() string {
	ps := `Add-Type -AssemblyName System.Windows.Forms;` +
		`$f = New-Object System.Windows.Forms.FolderBrowserDialog;` +
		`$f.Description = 'Escolha a pasta com os vídeos para editar';` +
		`$f.ShowNewFolderButton = $false;` +
		`$w = New-Object System.Windows.Forms.Form; $w.TopMost = $true;` +
		`if ($f.ShowDialog($w) -eq 'OK') { [Console]::OutputEncoding = [Text.Encoding]::UTF8; Write-Output $f.SelectedPath }`
	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(string(out), "\uFEFF"))
}

func openFolder(p string) { exec.Command("explorer", p).Start() }
