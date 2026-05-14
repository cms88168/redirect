//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

var (
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modShell32  = syscall.NewLazyDLL("shell32.dll")

	procGetModuleHandleW    = modKernel32.NewProc("GetModuleHandleW")
	procGetConsoleWindow    = modKernel32.NewProc("GetConsoleWindow")
	procAttachConsole       = modKernel32.NewProc("AttachConsole")
	procSetStdHandle        = modKernel32.NewProc("SetStdHandle")
	procRegisterClassExW    = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW     = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW      = modUser32.NewProc("DefWindowProcW")
	procGetMessageW         = modUser32.NewProc("GetMessageW")
	procTranslateMessage    = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW    = modUser32.NewProc("DispatchMessageW")
	procPostQuitMessage     = modUser32.NewProc("PostQuitMessage")
	procLoadIconW           = modUser32.NewProc("LoadIconW")
	procLoadCursorW         = modUser32.NewProc("LoadCursorW")
	procCreatePopupMenu     = modUser32.NewProc("CreatePopupMenu")
	procAppendMenuW         = modUser32.NewProc("AppendMenuW")
	procTrackPopupMenu      = modUser32.NewProc("TrackPopupMenu")
	procGetCursorPos        = modUser32.NewProc("GetCursorPos")
	procSetForegroundWindow = modUser32.NewProc("SetForegroundWindow")
	procShowWindow          = modUser32.NewProc("ShowWindow")
	procDestroyMenu         = modUser32.NewProc("DestroyMenu")
	procMessageBoxW         = modUser32.NewProc("MessageBoxW")
	procShellNotifyIconW    = modShell32.NewProc("Shell_NotifyIconW")
)

const (
	wmApp       = 0x8000
	wmTrayIcon  = wmApp + 1
	wmCommand   = 0x0111
	wmDestroy   = 0x0002
	wmRButtonUp = 0x0205
	wmLButtonUp = 0x0202

	nimAdd    = 0
	nimDelete = 2

	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04

	swHide = 0

	mfString = 0x00000000

	tpmRightButton = 0x0002
	tpmBottomAlign = 0x0020

	idiApplication = 32512
	idcArrow       = 32512

	mbOK        = 0
	mbIconError = 0x10

	idMenuExit = 1001
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     syscall.Handle
	hIcon         syscall.Handle
	hCursor       syscall.Handle
	hbrBackground syscall.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       syscall.Handle
}

type notifyIconData struct {
	cbSize           uint32
	hWnd             syscall.Handle
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            syscall.Handle
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     syscall.Handle
}

type point struct {
	X, Y int32
}

type wmsg struct {
	HWnd     syscall.Handle
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

var (
	notifyData   notifyIconData
	trayHWND     syscall.Handle
	trayInitOnce sync.Once
)

func wndProc(hwnd syscall.Handle, uMsg uint32, wParam, lParam uintptr) uintptr {
	switch uMsg {
	case wmTrayIcon:
		// lParam 的低 16 位即鼠标消息
		mouseMsg := uint16(lParam)
		if mouseMsg == wmRButtonUp || mouseMsg == wmLButtonUp {
			showTrayMenu(hwnd)
		}
		return 0
	case wmCommand:
		if uint16(wParam) == idMenuExit {
			removeTrayIcon()
			procPostQuitMessage.Call(0)
			return 0
		}
	case wmDestroy:
		removeTrayIcon()
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(uMsg), wParam, lParam)
	return ret
}

func showTrayMenu(hwnd syscall.Handle) {
	hMenuRet, _, _ := procCreatePopupMenu.Call()
	if hMenuRet == 0 {
		return
	}
	exitText, _ := syscall.UTF16PtrFromString("退出(&X)")
	procAppendMenuW.Call(hMenuRet, mfString, uintptr(idMenuExit), uintptr(unsafe.Pointer(exitText)))

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(uintptr(hwnd))
	procTrackPopupMenu.Call(hMenuRet, tpmBottomAlign|tpmRightButton,
		uintptr(pt.X), uintptr(pt.Y), 0, uintptr(hwnd), 0)
	procDestroyMenu.Call(hMenuRet)
}

func addTrayIcon(hwnd syscall.Handle, tooltip string) error {
	hIcon, _, _ := procLoadIconW.Call(0, idiApplication)

	notifyData = notifyIconData{}
	notifyData.cbSize = uint32(unsafe.Sizeof(notifyData))
	notifyData.hWnd = hwnd
	notifyData.uID = 1
	notifyData.uFlags = nifMessage | nifIcon | nifTip
	notifyData.uCallbackMessage = wmTrayIcon
	notifyData.hIcon = syscall.Handle(hIcon)

	tt, _ := syscall.UTF16FromString(tooltip)
	n := len(tt)
	if n > len(notifyData.szTip) {
		n = len(notifyData.szTip)
	}
	copy(notifyData.szTip[:n], tt[:n])
	notifyData.szTip[len(notifyData.szTip)-1] = 0

	ret, _, err := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&notifyData)))
	if ret == 0 {
		return fmt.Errorf("Shell_NotifyIcon 添加失败: %v", err)
	}
	return nil
}

func removeTrayIcon() {
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&notifyData)))
}

func hideConsole() {
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd != 0 {
		procShowWindow.Call(hwnd, swHide)
	}
}

// AttachParentConsole 将当前 GUI 程序附加到父进程的控制台，
// 并把标准输出/错误重定向到该控制台，使 CLI 模式下日志能显示在终端。
func AttachParentConsole() {
	const attachParent = ^uintptr(0) // ATTACH_PARENT_PROCESS = (DWORD)-1
	r, _, _ := procAttachConsole.Call(attachParent)
	if r == 0 {
		return
	}

	const (
		stdOutputHandle = ^uintptr(0) - 10 // -11
		stdErrorHandle  = ^uintptr(0) - 11 // -12
		stdInputHandle  = ^uintptr(0) - 9  // -10
	)

	if out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0); err == nil {
		os.Stdout = out
		os.Stderr = out
		procSetStdHandle.Call(stdOutputHandle, out.Fd())
		procSetStdHandle.Call(stdErrorHandle, out.Fd())
		log.SetOutput(out)
	}
	if in, err := os.OpenFile("CONIN$", os.O_RDWR, 0); err == nil {
		os.Stdin = in
		procSetStdHandle.Call(stdInputHandle, in.Fd())
	}
}

func showFatal(msg string) {
	title, _ := syscall.UTF16PtrFromString("SOCKS5端口转发 - 错误")
	text, _ := syscall.UTF16PtrFromString(msg)
	procMessageBoxW.Call(0,
		uintptr(unsafe.Pointer(text)),
		uintptr(unsafe.Pointer(title)),
		mbOK|mbIconError)
}

// setupTrayLogFile 在托盘模式下将日志重定向到 exe 同目录下的 redirect.log
func setupTrayLogFile() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	logPath := filepath.Join(filepath.Dir(exe), "redirect.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	log.SetOutput(f)
}

// runWithTrayIcon 在 Windows 下以系统托盘方式运行转发器
func runWithTrayIcon(config *Config) {
	hideConsole()
	setupTrayLogFile()

	// 后台启动转发器
	go runForwarders(config)

	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("RedirectTrayWindowClass")
	hCursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	hIcon, _, _ := procLoadIconW.Call(0, idiApplication)

	wc := wndClassEx{
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     syscall.Handle(hInst),
		hIcon:         syscall.Handle(hIcon),
		hCursor:       syscall.Handle(hCursor),
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))

	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		log.Printf("RegisterClassEx 失败: %v", err)
		return
	}

	wndName, _ := syscall.UTF16PtrFromString("RedirectTray")
	hwndRet, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(wndName)),
		0, 0, 0, 0, 0,
		0, 0, hInst, 0,
	)
	if hwndRet == 0 {
		log.Printf("CreateWindowEx 失败")
		return
	}
	trayHWND = syscall.Handle(hwndRet)

	tooltip := buildTrayTooltip(config)
	if err := addTrayIcon(trayHWND, tooltip); err != nil {
		log.Printf("添加托盘图标失败: %v", err)
	}

	// 消息循环
	var m wmsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		ret := int32(r)
		if ret <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	os.Exit(0)
}

func buildTrayTooltip(config *Config) string {
	parts := []string{"SOCKS5端口转发"}
	if config.EnableTCP {
		parts = append(parts, "TCP: "+config.TCPListenAddr+" -> "+config.TCPRemoteAddr)
	}
	if config.EnableUDP {
		parts = append(parts, "UDP: "+config.UDPListenAddr+" -> "+config.UDPRemoteAddr)
	}
	parts = append(parts, "代理: "+config.ProxyAddr)
	s := ""
	for i, p := range parts {
		if i > 0 {
			s += "\n"
		}
		s += p
	}
	// tooltip 最多 127 字符
	if len([]rune(s)) > 120 {
		r := []rune(s)
		s = string(r[:120])
	}
	return s
}
