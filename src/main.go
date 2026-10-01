//go:build windows

package main

import (
    "path/filepath"
    "strings"
    "syscall"
    "unsafe"
)

const (
    WH_KEYBOARD_LL = 13
    HC_ACTION       = 0

    WM_DESTROY      = 0x0002
    WM_PAINT        = 0x000F
    WM_ERASEBKGND   = 0x0014
    WM_MOUSEMOVE    = 0x0200
    WM_LBUTTONDOWN  = 0x0201
    WM_LBUTTONUP    = 0x0202
    WM_RBUTTONUP    = 0x0205
    WM_KEYDOWN      = 0x0100
    WM_KEYUP        = 0x0101
    WM_SYSKEYDOWN   = 0x0104
    WM_SYSKEYUP     = 0x0105
    WM_APPCOMMAND   = 0x0319

    VK_ESCAPE = 0x1B

    APPCOMMAND_MEDIA_PAUSE = 47
    SW_MINIMIZE             = 6
    SW_SHOWNOACTIVATE       = 4

    WS_POPUP        = 0x80000000
    WS_EX_TOPMOST   = 0x00000008
    WS_EX_TOOLWINDOW = 0x00000080
    WS_EX_NOACTIVATE = 0x08000000

    SWP_NOSIZE     = 0x0001
    SWP_NOZORDER   = 0x0004
    SWP_NOACTIVATE = 0x0010

    PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
    ERROR_ALREADY_EXISTS              = 183

    PS_SOLID = 0
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
    _        uint32
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
    pBeginPaint       = user32.NewProc("BeginPaint")
    pEndPaint         = user32.NewProc("EndPaint")
    pGetCursorPos     = user32.NewProc("GetCursorPos")
    pGetWindowRect    = user32.NewProc("GetWindowRect")
    pSetWindowPos     = user32.NewProc("SetWindowPos")
    pSetCapture       = user32.NewProc("SetCapture")
    pReleaseCapture   = user32.NewProc("ReleaseCapture")
    pGetSystemMetrics = user32.NewProc("GetSystemMetrics")
    pLoadCursorW      = user32.NewProc("LoadCursorW")
    pSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")

    pSetWindowsHookExW = user32.NewProc("SetWindowsHookExW")
    pCallNextHookEx    = user32.NewProc("CallNextHookEx")
    pUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")

    pEnumWindows              = user32.NewProc("EnumWindows")
    pIsWindowVisible          = user32.NewProc("IsWindowVisible")
    pGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
    pSendMessageW             = user32.NewProc("SendMessageW")
    pShowWindowAsync          = user32.NewProc("ShowWindowAsync")

    pCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
    pCreatePen        = gdi32.NewProc("CreatePen")
    pSelectObject     = gdi32.NewProc("SelectObject")
    pEllipse          = gdi32.NewProc("Ellipse")
    pArc              = gdi32.NewProc("Arc")
    pDeleteObject     = gdi32.NewProc("DeleteObject")

    pGetModuleHandleW          = kernel32.NewProc("GetModuleHandleW")
    pOpenProcess               = kernel32.NewProc("OpenProcess")
    pQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
    pCloseHandle               = kernel32.NewProc("CloseHandle")
    pCreateMutexW              = kernel32.NewProc("CreateMutexW")
    pGetLastError              = kernel32.NewProc("GetLastError")
)

var (
    hHook uintptr
    keyboardCallback uintptr
    windowCallback uintptr
    escapeSuppressed bool

    dragging bool
    dragCursorStart POINT
    dragWindowStart RECT
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
        h,
        0,
        uintptr(unsafe.Pointer(&buf[0])),
        uintptr(unsafe.Pointer(&size)),
    )
    if ok == 0 || size == 0 {
        return ""
    }
    full := syscall.UTF16ToString(buf[:size])
    return strings.ToLower(filepath.Base(full))
}

func findVLCWindow() uintptr {
    var found uintptr

    cb := syscall.NewCallback(func(hwnd uintptr, lParam uintptr) uintptr {
        visible, _, _ := pIsWindowVisible.Call(hwnd)
        if visible == 0 {
            return 1
        }

        var pid uint32
        pGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
        if pid != 0 && processName(pid) == "vlc.exe" {
            found = hwnd
            return 0
        }
        return 1
    })

    pEnumWindows.Call(cb, 0)
    return found
}

func pauseAndMinimizeVLC(hwnd uintptr) {
    // Dedicated MEDIA_PAUSE command. Unlike play/pause toggle, this should not
    // resume playback if VLC is already paused.
    pSendMessageW.Call(
        hwnd,
        WM_APPCOMMAND,
        hwnd,
        uintptr(APPCOMMAND_MEDIA_PAUSE<<16),
    )
    pShowWindowAsync.Call(hwnd, SW_MINIMIZE)
}

func keyboardProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
    if nCode == HC_ACTION {
        kb := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))
        if kb.VkCode == VK_ESCAPE {
            switch uint32(wParam) {
            case WM_KEYDOWN, WM_SYSKEYDOWN:
                if escapeSuppressed {
                    return 1
                }
                if hwnd := findVLCWindow(); hwnd != 0 {
                    escapeSuppressed = true
                    pauseAndMinimizeVLC(hwnd)
                    return 1
                }
            case WM_KEYUP, WM_SYSKEYUP:
                if escapeSuppressed {
                    escapeSuppressed = false
                    return 1
                }
            }
        }
    }

    ret, _, _ := pCallNextHookEx.Call(hHook, uintptr(nCode), wParam, lParam)
    return ret
}

func paintSmiley(hwnd uintptr) {
    var ps PAINTSTRUCT
    hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
    if hdc == 0 {
        return
    }
    defer pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

    yellowBrush, _, _ := pCreateSolidBrush.Call(rgb(255, 216, 77))
    blackBrush, _, _ := pCreateSolidBrush.Call(rgb(32, 32, 32))
    blackPen, _, _ := pCreatePen.Call(PS_SOLID, 1, rgb(32, 32, 32))

    defer pDeleteObject.Call(yellowBrush)
    defer pDeleteObject.Call(blackBrush)
    defer pDeleteObject.Call(blackPen)

    oldBrush, _, _ := pSelectObject.Call(hdc, yellowBrush)
    oldPen, _, _ := pSelectObject.Call(hdc, blackPen)

    pEllipse.Call(hdc, 1, 1, 19, 19)

    pSelectObject.Call(hdc, blackBrush)
    pEllipse.Call(hdc, 5, 6, 8, 9)
    pEllipse.Call(hdc, 12, 6, 15, 9)

    // Smile arc.
    pArc.Call(hdc, 5, 7, 15, 16, 5, 10, 15, 10)

    pSelectObject.Call(hdc, oldBrush)
    pSelectObject.Call(hdc, oldPen)
}

func windowProc(hwnd uintptr, msg uint32, wParam uintptr, lParam uintptr) uintptr {
    switch msg {
    case WM_PAINT:
        paintSmiley(hwnd)
        return 0

    case WM_ERASEBKGND:
        return 1

    case WM_LBUTTONDOWN:
        dragging = true
        pGetCursorPos.Call(uintptr(unsafe.Pointer(&dragCursorStart)))
        pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&dragWindowStart)))
        pSetCapture.Call(hwnd)
        return 0

    case WM_MOUSEMOVE:
        if dragging {
            var p POINT
            pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
            x := dragWindowStart.Left + (p.X - dragCursorStart.X)
            y := dragWindowStart.Top + (p.Y - dragCursorStart.Y)
            pSetWindowPos.Call(
                hwnd,
                0,
                uintptr(x),
                uintptr(y),
                0,
                0,
                SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE,
            )
        }
        return 0

    case WM_LBUTTONUP:
        if dragging {
            dragging = false
            pReleaseCapture.Call()
        }
        return 0

    case WM_RBUTTONUP:
        pDestroyWindow.Call(hwnd)
        return 0

    case WM_DESTROY:
        pPostQuitMessage.Call(0)
        return 0
    }

    ret, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
    return ret
}

func alreadyRunning() bool {
    name, _ := syscall.UTF16PtrFromString("Local\\VLC_ESC_Smiley_20x20")
    h, _, _ := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
    if h == 0 {
        return false
    }
    err, _, _ := pGetLastError.Call()
    return err == ERROR_ALREADY_EXISTS
}

func main() {
    if alreadyRunning() {
        return
    }

    pSetProcessDPIAware.Call()

    hInstance, _, _ := pGetModuleHandleW.Call(0)
    className, _ := syscall.UTF16PtrFromString("VLCESCSmileyWindow")
    title, _ := syscall.UTF16PtrFromString("VLC ESC Smiley")

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
        WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE,
        uintptr(unsafe.Pointer(className)),
        uintptr(unsafe.Pointer(title)),
        WS_POPUP,
        uintptr(x),
        35,
        20,
        20,
        0,
        0,
        hInstance,
        0,
    )
    if hwnd == 0 {
        return
    }

    keyboardCallback = syscall.NewCallback(keyboardProc)
    hHook, _, _ = pSetWindowsHookExW.Call(
        WH_KEYBOARD_LL,
        keyboardCallback,
        hInstance,
        0,
    )
    if hHook == 0 {
        pDestroyWindow.Call(hwnd)
        return
    }
    defer pUnhookWindowsHookEx.Call(hHook)

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
}
