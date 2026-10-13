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
	"unsafe"

	"github.com/ulikunitz/xz"
)

//go:embed assets/ffmpeg.exe.xz
var ffmpegXZ []byte

//go:embed assets/ffmpeg.size
var ffmpegSize string

const (
	createNoWindow   = 0x08000000
	belowNormalClass = 0x00004000
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	ntdll    = syscall.NewLazyDLL("ntdll.dll")
)

// Descompacta o ffmpeg uma vez em %LOCALAPPDATA%\Zaptaku e reaproveita.
func prepareFFmpeg(tmp string) (string, error) {
	want, _ := strconv.ParseInt(strings.TrimSpace(ffmpegSize), 10, 64)
	p := filepath.Join(appDir(), "ffmpeg-6.0.exe")
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

func hideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

func lowPriority(cmd *exec.Cmd, on bool) {
	if !on {
		return
	}
	hideWindow(cmd)
	cmd.SysProcAttr.CreationFlags |= belowNormalClass
}

func pickFolder(title string) string {
	ps := `Add-Type -AssemblyName System.Windows.Forms;` +
		`$f = New-Object System.Windows.Forms.FolderBrowserDialog;` +
		`$f.Description = '` + strings.ReplaceAll(title, "'", "") + `';` +
		`$f.ShowNewFolderButton = $true;` +
		`$w = New-Object System.Windows.Forms.Form; $w.TopMost = $true;` +
		`if ($f.ShowDialog($w) -eq 'OK') { [Console]::OutputEncoding = [Text.Encoding]::UTF8; Write-Output $f.SelectedPath }`
	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-Command", ps)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(string(out), "\uFEFF"))
}

func openFolder(p string) { exec.Command("explorer", p).Start() }

// Abre a interface numa janela própria (Edge ou Chrome em modo app); senão, no navegador padrão.
func openApp(url string) {
	args := []string{"--app=" + url, "--window-size=1360,900", "--user-data-dir=" + filepath.Join(appDir(), "janela")}
	for _, b := range []string{
		os.Getenv("ProgramFiles(x86)") + `\Microsoft\Edge\Application\msedge.exe`,
		os.Getenv("ProgramFiles") + `\Microsoft\Edge\Application\msedge.exe`,
		os.Getenv("ProgramFiles") + `\Google\Chrome\Application\chrome.exe`,
		os.Getenv("LocalAppData") + `\Google\Chrome\Application\chrome.exe`,
	} {
		if _, err := os.Stat(b); err == nil {
			if exec.Command(b, args...).Start() == nil {
				return
			}
		}
	}
	exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func shutdownPC() {
	exec.Command("shutdown", "/s", "/t", "60", "/c", "Zaptaku Editor terminou. Para cancelar: shutdown /a").Start()
}

func fatalBox(msg string) {
	t, _ := syscall.UTF16PtrFromString("Zaptaku Editor")
	m, _ := syscall.UTF16PtrFromString(msg)
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10)
	os.Exit(1)
}

// ---------- pausar / retomar ----------

func withHandle(p *os.Process, name string) {
	h, err := syscall.OpenProcess(0x0800, false, uint32(p.Pid)) // PROCESS_SUSPEND_RESUME
	if err != nil {
		return
	}
	defer syscall.CloseHandle(h)
	ntdll.NewProc(name).Call(uintptr(h))
}

func suspendProcess(p *os.Process) { withHandle(p, "NtSuspendProcess") }
func resumeProcess(p *os.Process)  { withHandle(p, "NtResumeProcess") }

// ---------- sensores ----------

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

// Temperatura pelas zonas térmicas do Windows, num PowerShell que fica rodando.
func cpuTemp() int {
	if v, ok := readFakeInt("ZAPTAKU_FAKE_CPU"); ok {
		return v
	}
	cpuOnce.Do(func() {
		cpuT.Store(-1)
		ps := `while($true){ $t=$null;` +
			`try{ $t=(Get-CimInstance -Namespace root/cimv2 -ClassName Win32_PerfFormattedData_Counters_ThermalZoneInformation -ErrorAction Stop | Measure-Object -Property HighPrecisionTemperature -Maximum).Maximum }catch{};` +
			`if(-not $t){ try{ $t=(Get-CimInstance -Namespace root/wmi -ClassName MSAcpi_ThermalZoneTemperature -ErrorAction Stop | Measure-Object -Property CurrentTemperature -Maximum).Maximum }catch{} };` +
			`if($t){ [Console]::Out.WriteLine([int]($t/10 - 273.15)) } else { [Console]::Out.WriteLine(-1) };` +
			`[Console]::Out.Flush(); Start-Sleep -Seconds 3 }`
		cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
		hideWindow(cmd)
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

type filetime struct{ lo, hi uint32 }

func (f filetime) u() uint64 { return uint64(f.hi)<<32 | uint64(f.lo) }

var lastIdle, lastTotal uint64

func cpuUsage() int {
	var idle, kern, usr filetime
	r, _, _ := kernel32.NewProc("GetSystemTimes").Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kern)), uintptr(unsafe.Pointer(&usr)))
	if r == 0 {
		return 0
	}
	total := kern.u() + usr.u() // kernel já inclui idle
	di, dt := idle.u()-lastIdle, total-lastTotal
	lastIdle, lastTotal = idle.u(), total
	if dt == 0 || dt == total {
		return 0
	}
	return int(100 * (1 - float64(di)/float64(dt)))
}

type memStatus struct {
	Length, MemoryLoad                           uint32
	TotalPhys, AvailPhys, TotalPage, AvailPage   uint64
	TotalVirtual, AvailVirtual, AvailExtendedVir uint64
}

func ramGB() (float64, float64) {
	m := memStatus{Length: uint32(unsafe.Sizeof(memStatus{}))}
	r, _, _ := kernel32.NewProc("GlobalMemoryStatusEx").Call(uintptr(unsafe.Pointer(&m)))
	if r == 0 {
		return 0, 0
	}
	g := float64(1 << 30)
	return float64(m.TotalPhys-m.AvailPhys) / g, float64(m.TotalPhys) / g
}
