//go:build windows

package main

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/ulikunitz/xz"
)

//go:embed assets/ffmpeg.exe.xz
var ffmpegXZ []byte

//go:embed assets/ffmpeg.size
var ffmpegSize string

// Descompacta o ffmpeg uma vez para %LOCALAPPDATA%\Zaptaku e reaproveita nas próximas execuções.
func prepareFFmpeg(tmp string) (string, error) {
	want, _ := strconv.ParseInt(strings.TrimSpace(ffmpegSize), 10, 64)
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = tmp
	}
	dir = filepath.Join(dir, "Zaptaku")
	os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, "ffmpeg-6.0.exe")
	if st, err := os.Stat(p); err == nil && st.Size() == want {
		return p, nil
	}
	r, err := xz.NewReader(bytes.NewReader(ffmpegXZ))
	if err != nil {
		return "", err
	}
	part := p + ".part"
	f, err := os.Create(part)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, r)
	f.Close()
	if err != nil || n != want {
		os.Remove(part)
		return "", fmt.Errorf("falha ao descompactar o ffmpeg (%v, %d bytes)", err, n)
	}
	os.Remove(p)
	return p, os.Rename(part, p)
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

// ---------- pausar / retomar processos (trava de temperatura) ----------

var (
	ntdll       = syscall.NewLazyDLL("ntdll.dll")
	ntSuspend   = ntdll.NewProc("NtSuspendProcess")
	ntResume    = ntdll.NewProc("NtResumeProcess")
	procSuspRes = uint32(0x0800) // PROCESS_SUSPEND_RESUME
)

func withHandle(p *os.Process, fn *syscall.LazyProc) {
	h, err := syscall.OpenProcess(procSuspRes, false, uint32(p.Pid))
	if err != nil {
		return
	}
	defer syscall.CloseHandle(h)
	fn.Call(uintptr(h))
}

func suspendProcess(p *os.Process) { withHandle(p, ntSuspend) }
func resumeProcess(p *os.Process)  { withHandle(p, ntResume) }

// ---------- temperaturas ----------

func nvidiaSmi() string {
	if p, err := exec.LookPath("nvidia-smi"); err == nil {
		return p
	}
	for _, p := range []string{`C:\Windows\System32\nvidia-smi.exe`, `C:\Program Files\NVIDIA Corporation\NVSMI\nvidia-smi.exe`} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

var cpuT atomic.Int64
var cpuOnce sync.Once

// Lê a temperatura pelo Windows (zonas térmicas ACPI) num processo do PowerShell que fica rodando.
func cpuTemp() int {
	cpuOnce.Do(func() {
		cpuT.Store(-1)
		ps := `while($true){ $t=$null;` +
			`try{ $t=(Get-CimInstance -Namespace root/cimv2 -ClassName Win32_PerfFormattedData_Counters_ThermalZoneInformation -ErrorAction Stop | Measure-Object -Property HighPrecisionTemperature -Maximum).Maximum }catch{};` +
			`if(-not $t){ try{ $t=(Get-CimInstance -Namespace root/wmi -ClassName MSAcpi_ThermalZoneTemperature -ErrorAction Stop | Measure-Object -Property CurrentTemperature -Maximum).Maximum }catch{} };` +
			`if($t){ [Console]::Out.WriteLine([int]($t/10 - 273.15)) } else { [Console]::Out.WriteLine(-1) };` +
			`[Console]::Out.Flush(); Start-Sleep -Seconds 4 }`
		cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
		out, err := cmd.StdoutPipe()
		if err != nil || cmd.Start() != nil {
			return
		}
		registerCleanup(func() { cmd.Process.Kill() })
		go func() {
			sc := bufio.NewScanner(out)
			for sc.Scan() {
				v, err := strconv.Atoi(strings.TrimSpace(sc.Text()))
				if err != nil || v < 1 || v > 125 {
					v = -1
				}
				cpuT.Store(int64(v))
			}
		}()
	})
	return int(cpuT.Load())
}
