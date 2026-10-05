//go:build windows

package main

import (
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strings"
    "sync/atomic"
    "syscall"
    "unsafe"
)

const (
    appVersion = "2.0.0"

    // Window messages
    WM_DESTROY       = 0x0002
    WM_CLOSE         = 0x0010
    WM_PAINT         = 0x000F
    WM_ERASEBKGND    = 0x0014
    WM_NCHITTEST     = 0x0084
    WM_KEYDOWN       = 0x0100
    WM_KEYUP         = 0x0101
    WM_SYSKEYDOWN    = 0x0104
    WM_SYSKEYUP      = 0x0105
    WM_NCLBUTTONDOWN = 0x00A1
    WM_LBUTTONDOWN   = 0x0201
    WM_RBUTTONDOWN   = 0x0204
    WM_APPCOMMAND    = 0x0319
    WM_QUIT          = 0x0012

    HTCLIENT  = 1
    HTCAPTION = 2

    // Keyboard hook
    WH_KEYBOARD_LL = 13
    HC_ACTION      = 0
    VK_ESCAPE      = 0x1B

    // VLC / window commands
    APPCOMMAND_MEDIA_PAUSE = 47
    SW_MINIMIZE            = 6
    SW_SHOWNOACTIVATE      = 4

    // Window styles
    WS_POPUP         = 0x80000000
    WS_EX_TOPMOST    = 0x00000008
    WS_EX_TOOLWINDOW = 0x00000080

    // Process access
    PROCESS_QUERY_LIMITED_INFORMATION = 0x1000

    // Drawing
    PS_SOLID = 0

    // Screen metric
    SM_CXSCREEN = 0
)

type POINT struct {
    X int32
    Y int32
}

type RECT struct {
    Left   int32
    Top    int32
    Right  int32
    Bottom int32
}

type MSG struct {
    Hwnd     uintptr
    Message  uint32
    _        uint32 // alignment padding on 64-bit Windows
    WParam   uintptr
    LParam   uintptr
    Time     uint32
    Pt       POINT
    LPrivate uint32
}

type WNDCLASSEX struct {
    CbSize        uint32
    Style         uint32
    LpfnWndProc   uintptr
    CbClsExtra    int32
    CbWndExtra    int32
    HInstance     uintptr
    HIcon         uintptr
    HCursor       uintptr
    HbrBackground uintptr
    LpszMenuName  *uint16
    LpszClassName *uint16
    HIconSm       uintptr
}

type PAINTSTRUCT struct {
    Hdc         uintptr
    FErase      int32
    RcPaint     RECT
    FRestore    int32
    FIncUpdate  int32
    RgbReserved [32]byte
}

type KBDLLHOOKSTRUCT struct {
    VkCode      uint32
    ScanCode    uint32
    Flags       uint32
    Time        uint32
    DwExtraInfo uintptr
}

var (
    user32   = syscall.NewLazyDLL("user32.dll")
    kernel32 = syscall.NewLazyDLL("kernel32.dll")
    gdi32    = syscall.NewLazyDLL("gdi32.dll")

    pRegisterClassExW = user32.NewProc("RegisterClassExW")
    pCreateWindowExW  = user32.NewProc("CreateWindowExW")
    pDefWindowProcW   = user32.NewProc("DefWindowProcW")
    pShowWindow       = user32.NewProc("ShowWindow")
    pUpdateWindow     = user32.NewProc("UpdateWindow")
    pGetMessageW      = user32.NewProc("GetMessageW")
    pTranslateMessage = user32.NewProc("TranslateMessage")
    pDispatchMessageW = user32.NewProc("DispatchMessageW")
    pPostQuitMessage  = user32.NewProc("PostQuitMessage")
    pDestroyWindow    = user32.NewProc("DestroyWindow")
    pSendMessageW     = user32.NewProc("SendMessageW")
    pReleaseCapture   = user32.NewProc("ReleaseCapture")
    pBeginPaint       = user32.NewProc("BeginPaint")
    pEndPaint         = user32.NewProc("EndPaint")
    pLoadCursorW      = user32.NewProc("LoadCursorW")
    pSendNotifyMsgW   = user32.NewProc("SendNotifyMessageW")
    pShowWindowAsync  = user32.NewProc("ShowWindowAsync")
    pSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
    pGetSystemMetrics = user32.NewProc("GetSystemMetrics")

    pSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
    pCallNextHookEx      = user32.NewProc("CallNextHookEx")
    pUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
    pPostThreadMessageW  = user32.NewProc("PostThreadMessageW")

    pEnumWindows              = user32.NewProc("EnumWindows")
    pIsWindowVisible          = user32.NewProc("IsWindowVisible")
    pGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
    pGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")

    pCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
    pCreatePen        = gdi32.NewProc("CreatePen")
    pSelectObject     = gdi32.NewProc("SelectObject")
    pEllipse          = gdi32.NewProc("Ellipse")
    pArc              = gdi32.NewProc("Arc")
    pRectangle        = gdi32.NewProc("Rectangle")
    pDeleteObject     = gdi32.NewProc("DeleteObject")

    pGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
    pOpenProcess                = kernel32.NewProc("OpenProcess")
    pQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
    pCloseHandle                = kernel32.NewProc("CloseHandle")
    pCreateMutexW               = kernel32.NewProc("CreateMutexW")
    pGetLastError               = kernel32.NewProc("GetLastError")
    pGetCurrentThreadId         = kernel32.NewProc("GetCurrentThreadId")
)

var (
    windowCallback   uintptr
    keyboardCallback uintptr
    hookHandle       uintptr
    hookThreadID     atomic.Uint32
    escapeDown       atomic.Bool
    appClosing       atomic.Bool
    singletonMutex   uintptr
)

func rgb(r, g, b byte) uintptr {
    return uintptr(uint32(r) | uint32(g)<<8 | uint32(b)<<16)
}

func processName(pid uint32) string {
    h, _, _ := pOpenProcess.Call(PROCESS_QUERY_LIMITED_INFORMATION, 0, uintptr(pid))
    if h == 0 {
        return ""
    }
    defer pCloseHandle.Call(h)

    buf := make([]uint16, 32768)
    size := uint32(len(buf))
    ok, _, _ := pQueryFullProcessImageNameW.Call(
        h, 0,
        uintptr(unsafe.Pointer(&buf[0])),
        uintptr(unsafe.Pointer(&size)),
    )
    if ok == 0 || size == 0 {
        return ""
    }
    return strings.ToLower(filepath.Base(syscall.UTF16ToString(buf[:size])))
}

// Find the most likely main visible VLC window. Prefer a window with a title.
func findVLCWindow() uintptr {
    var titled uintptr
    var fallback uintptr

    cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
        visible, _, _ := pIsWindowVisible.Call(hwnd)
        if visible == 0 {
            return 1
        }

        var pid uint32
        pGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
        if pid == 0 || processName(pid) != "vlc.exe" {
            return 1
        }

        if fallback == 0 {
            fallback = hwnd
        }
        titleLen, _, _ := pGetWindowTextLengthW.Call(hwnd)
        if titleLen > 0 {
            titled = hwnd
            return 0
        }
        return 1
    })

    pEnumWindows.Call(cb, 0)
    if titled != 0 {
        return titled
    }
    return fallback
}

func pauseAndMinimizeVLC(hwnd uintptr) {
    // Asynchronous pause notification so a hung VLC cannot stall our hook thread.
    pSendNotifyMsgW.Call(hwnd, WM_APPCOMMAND, hwnd, uintptr(APPCOMMAND_MEDIA_PAUSE<<16))
    pShowWindowAsync.Call(hwnd, SW_MINIMIZE)
}

func keyboardProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
    if nCode == HC_ACTION && !appClosing.Load() {
        kb := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))
        if kb.VkCode == VK_ESCAPE {
            switch uint32(wParam) {
            case WM_KEYDOWN, WM_SYSKEYDOWN:
                if escapeDown.Load() {
                    return 1
                }
                if hwnd := findVLCWindow(); hwnd != 0 {
                    escapeDown.Store(true)
                    pauseAndMinimizeVLC(hwnd)
                    return 1
                }
            case WM_KEYUP, WM_SYSKEYUP:
                if escapeDown.Swap(false) {
                    return 1
                }
            }
        }
    }

    ret, _, _ := pCallNextHookEx.Call(hookHandle, uintptr(nCode), wParam, lParam)
    return ret
}

func keyboardHookLoop(hInstance uintptr, ready chan<- bool) {
    runtime.LockOSThread()
    defer runtime.UnlockOSThread()

    tid, _, _ := pGetCurrentThreadId.Call()
    hookThreadID.Store(uint32(tid))

    keyboardCallback = syscall.NewCallback(keyboardProc)
    h, _, _ := pSetWindowsHookExW.Call(WH_KEYBOARD_LL, keyboardCallback, hInstance, 0)
    if h == 0 {
        ready <- false
        return
    }
    hookHandle = h
    ready <- true

    var msg MSG
    for {
        ret, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
        if int32(ret) <= 0 {
            break
        }
        // Low-level keyboard hooks only need a live message loop.
    }

    pUnhookWindowsHookEx.Call(h)
    hookHandle = 0
}

func stopKeyboardHook() {
    appClosing.Store(true)
    if tid := hookThreadID.Load(); tid != 0 {
        pPostThreadMessageW.Call(uintptr(tid), WM_QUIT, 0, 0)
    }
}

func paintSmiley(hwnd uintptr) {
    var ps PAINTSTRUCT
    hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
    if hdc == 0 {
        return
    }
    defer pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

    // White background makes the full 20x20 hit area obvious and reliable.
    whiteBrush, _, _ := pCreateSolidBrush.Call(rgb(255, 255, 255))
    yellowBrush, _, _ := pCreateSolidBrush.Call(rgb(255, 216, 77))
    blackBrush, _, _ := pCreateSolidBrush.Call(rgb(32, 32, 32))
    blackPen, _, _ := pCreatePen.Call(PS_SOLID, 1, rgb(32, 32, 32))
    whitePen, _, _ := pCreatePen.Call(PS_SOLID, 1, rgb(255, 255, 255))

    defer pDeleteObject.Call(whiteBrush)
    defer pDeleteObject.Call(yellowBrush)
    defer pDeleteObject.Call(blackBrush)
    defer pDeleteObject.Call(blackPen)
    defer pDeleteObject.Call(whitePen)

    oldBrush, _, _ := pSelectObject.Call(hdc, whiteBrush)
    oldPen, _, _ := pSelectObject.Call(hdc, whitePen)
    pRectangle.Call(hdc, 0, 0, 20, 20)

    pSelectObject.Call(hdc, yellowBrush)
    pSelectObject.Call(hdc, blackPen)
    pEllipse.Call(hdc, 1, 1, 19, 19)

    pSelectObject.Call(hdc, blackBrush)
    pEllipse.Call(hdc, 5, 6, 8, 9)
    pEllipse.Call(hdc, 12, 6, 15, 9)
    pArc.Call(hdc, 5, 7, 15, 16, 5, 10, 15, 10)

    pSelectObject.Call(hdc, oldBrush)
    pSelectObject.Call(hdc, oldPen)
}

func windowProc(hwnd uintptr, msg uint32, wParam uintptr, lParam uintptr) uintptr {
    switch msg {
    case WM_NCHITTEST:
        return HTCLIENT

    case WM_PAINT:
        paintSmiley(hwnd)
        return 0

    case WM_ERASEBKGND:
        return 1

    case WM_LBUTTONDOWN:
        // Let Windows perform its own native window-move modal loop.
        // This avoids a custom WM_MOUSEMOVE drag implementation entirely.
        pReleaseCapture.Call()
        pSendMessageW.Call(hwnd, WM_NCLBUTTONDOWN, HTCAPTION, 0)
        return 0

    case WM_RBUTTONDOWN:
        pDestroyWindow.Call(hwnd)
        return 0

    case WM_CLOSE:
        pDestroyWindow.Call(hwnd)
        return 0

    case WM_DESTROY:
        stopKeyboardHook()
        pPostQuitMessage.Call(0)
        return 0
    }

    ret, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
    return ret
}

func killKnownOldBuilds() {
    // Older builds could leave behind an unresponsive UI thread. Kill only the
    // exact filenames shipped by earlier versions, never wildcard processes.
    oldNames := []string{
        "VLC_ESC_Smiley.exe",
        "VLC_ESC_Smiley_v1.1.exe",
        "VLC_ESC_Smiley_v1.2.exe",
        "VLC_ESC_Smiley_v1.3.exe",
        "VLC_ESC_Smiley_v1.4.exe",
    }
    current := strings.ToLower(filepath.Base(os.Args[0]))
    for _, name := range oldNames {
        if strings.ToLower(name) == current {
            continue
        }
        cmd := exec.Command("taskkill.exe", "/F", "/IM", name)
        cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
        _ = cmd.Run()
    }
}

func acquireSingleton() bool {
    name, _ := syscall.UTF16PtrFromString("Local\\VLC_ESC_Smiley_20x20_v2")
    h, _, _ := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
    if h == 0 {
        return false
    }
    singletonMutex = h
    // ERROR_ALREADY_EXISTS = 183
    errCode, _, _ := pGetLastError.Call()
    return errCode != 183
}

func main() {
    // CRITICAL: Win32 windows and their message queue are thread-affine.
    // Keep this goroutine on one OS thread from before window creation until
    // after the GUI message loop exits.
    runtime.LockOSThread()
    defer runtime.UnlockOSThread()

    killKnownOldBuilds()
    if !acquireSingleton() {
        return
    }
    defer func() {
        if singletonMutex != 0 {
            pCloseHandle.Call(singletonMutex)
        }
    }()

    pSetProcessDPIAware.Call()

    hInstance, _, _ := pGetModuleHandleW.Call(0)
    className, _ := syscall.UTF16PtrFromString("VLCESCSmileyWindowV2")
    title, _ := syscall.UTF16PtrFromString("VLC ESC Smiley " + appVersion)

    windowCallback = syscall.NewCallback(windowProc)
    cursor, _, _ := pLoadCursorW.Call(0, 32512) // IDC_ARROW

    wc := WNDCLASSEX{
        CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
        LpfnWndProc:   windowCallback,
        HInstance:     hInstance,
        HCursor:       cursor,
        LpszClassName: className,
    }

    atom, _, _ := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
    if atom == 0 {
        return
    }

    screenW, _, _ := pGetSystemMetrics.Call(SM_CXSCREEN)
    x := int32(screenW) - 35
    if x < 0 {
        x = 0
    }

    hwnd, _, _ := pCreateWindowExW.Call(
        WS_EX_TOPMOST|WS_EX_TOOLWINDOW,
        uintptr(unsafe.Pointer(className)),
        uintptr(unsafe.Pointer(title)),
        WS_POPUP,
        uintptr(x), 35,
        20, 20,
        0, 0,
        hInstance,
        0,
    )
    if hwnd == 0 {
        return
    }

    ready := make(chan bool, 1)
    go keyboardHookLoop(hInstance, ready)
    if ok := <-ready; !ok {
        pDestroyWindow.Call(hwnd)
        return
    }

    pShowWindow.Call(hwnd, SW_SHOWNOACTIVATE)
    pUpdateWindow.Call(hwnd)

    var msg MSG
    for {
        ret, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
        if int32(ret) <= 0 {
            break
        }
        pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
        pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
    }

    stopKeyboardHook()
}
