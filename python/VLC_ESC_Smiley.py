# -*- coding: utf-8 -*-
"""
VLC ESC Smiley
Windows-only, external Python packages are not required.

Behavior:
- 20x20 frameless smiley, always on top
- drag with left mouse button
- right-click exits
- while running, global ESC:
    * if a VLC window exists, send PAUSE and minimize VLC
    * ESC is swallowed only when VLC was found
    * if VLC is not running, ESC works normally
"""

import ctypes
import os
import threading
import tkinter as tk
from ctypes import wintypes

if os.name != "nt":
    raise SystemExit("Ez a program csak Windowson fut.")

# ---------------- Windows constants ----------------
WH_KEYBOARD_LL = 13
HC_ACTION = 0

WM_KEYDOWN = 0x0100
WM_KEYUP = 0x0101
WM_SYSKEYDOWN = 0x0104
WM_SYSKEYUP = 0x0105
WM_QUIT = 0x0012
WM_APPCOMMAND = 0x0319

VK_ESCAPE = 0x1B

APPCOMMAND_MEDIA_PAUSE = 47
SW_MINIMIZE = 6

PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
ERROR_ALREADY_EXISTS = 183

user32 = ctypes.WinDLL("user32", use_last_error=True)
kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)

LRESULT = ctypes.c_ssize_t
ULONG_PTR = ctypes.c_size_t


class KBDLLHOOKSTRUCT(ctypes.Structure):
    _fields_ = [
        ("vkCode", wintypes.DWORD),
        ("scanCode", wintypes.DWORD),
        ("flags", wintypes.DWORD),
        ("time", wintypes.DWORD),
        ("dwExtraInfo", ULONG_PTR),
    ]


HOOKPROC = ctypes.WINFUNCTYPE(
    LRESULT, ctypes.c_int, wintypes.WPARAM, wintypes.LPARAM
)
ENUMWINDOWSPROC = ctypes.WINFUNCTYPE(
    wintypes.BOOL, wintypes.HWND, wintypes.LPARAM
)

user32.SetWindowsHookExW.argtypes = [
    ctypes.c_int, HOOKPROC, wintypes.HINSTANCE, wintypes.DWORD
]
user32.SetWindowsHookExW.restype = wintypes.HANDLE

user32.CallNextHookEx.argtypes = [
    wintypes.HANDLE, ctypes.c_int, wintypes.WPARAM, wintypes.LPARAM
]
user32.CallNextHookEx.restype = LRESULT

user32.UnhookWindowsHookEx.argtypes = [wintypes.HANDLE]
user32.UnhookWindowsHookEx.restype = wintypes.BOOL

user32.GetMessageW.argtypes = [
    ctypes.POINTER(wintypes.MSG), wintypes.HWND, wintypes.UINT, wintypes.UINT
]
user32.GetMessageW.restype = wintypes.BOOL

user32.PostThreadMessageW.argtypes = [
    wintypes.DWORD, wintypes.UINT, wintypes.WPARAM, wintypes.LPARAM
]
user32.PostThreadMessageW.restype = wintypes.BOOL

user32.EnumWindows.argtypes = [ENUMWINDOWSPROC, wintypes.LPARAM]
user32.EnumWindows.restype = wintypes.BOOL

user32.IsWindowVisible.argtypes = [wintypes.HWND]
user32.IsWindowVisible.restype = wintypes.BOOL

user32.GetWindowTextLengthW.argtypes = [wintypes.HWND]
user32.GetWindowTextLengthW.restype = ctypes.c_int

user32.GetWindowTextW.argtypes = [wintypes.HWND, wintypes.LPWSTR, ctypes.c_int]
user32.GetWindowTextW.restype = ctypes.c_int

user32.GetWindowThreadProcessId.argtypes = [
    wintypes.HWND, ctypes.POINTER(wintypes.DWORD)
]
user32.GetWindowThreadProcessId.restype = wintypes.DWORD

user32.SendMessageW.argtypes = [
    wintypes.HWND, wintypes.UINT, wintypes.WPARAM, wintypes.LPARAM
]
user32.SendMessageW.restype = LRESULT

user32.ShowWindowAsync.argtypes = [wintypes.HWND, ctypes.c_int]
user32.ShowWindowAsync.restype = wintypes.BOOL

kernel32.OpenProcess.argtypes = [
    wintypes.DWORD, wintypes.BOOL, wintypes.DWORD
]
kernel32.OpenProcess.restype = wintypes.HANDLE

kernel32.QueryFullProcessImageNameW.argtypes = [
    wintypes.HANDLE,
    wintypes.DWORD,
    wintypes.LPWSTR,
    ctypes.POINTER(wintypes.DWORD),
]
kernel32.QueryFullProcessImageNameW.restype = wintypes.BOOL

kernel32.CloseHandle.argtypes = [wintypes.HANDLE]
kernel32.CloseHandle.restype = wintypes.BOOL

kernel32.GetCurrentThreadId.restype = wintypes.DWORD
kernel32.GetModuleHandleW.argtypes = [wintypes.LPCWSTR]
kernel32.GetModuleHandleW.restype = wintypes.HMODULE

# Prevent accidental multiple copies.
kernel32.CreateMutexW.argtypes = [ctypes.c_void_p, wintypes.BOOL, wintypes.LPCWSTR]
kernel32.CreateMutexW.restype = wintypes.HANDLE
_single_instance_mutex = kernel32.CreateMutexW(
    None, False, "Local\\VLC_ESC_Smiley_20x20"
)
if ctypes.get_last_error() == ERROR_ALREADY_EXISTS:
    raise SystemExit(0)


def get_process_name(pid: int) -> str:
    handle = kernel32.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, False, pid)
    if not handle:
        return ""

    try:
        size = wintypes.DWORD(32768)
        buf = ctypes.create_unicode_buffer(size.value)
        if kernel32.QueryFullProcessImageNameW(handle, 0, buf, ctypes.byref(size)):
            return os.path.basename(buf.value).lower()
        return ""
    finally:
        kernel32.CloseHandle(handle)


def get_window_title(hwnd) -> str:
    length = user32.GetWindowTextLengthW(hwnd)
    if length <= 0:
        return ""
    buf = ctypes.create_unicode_buffer(length + 1)
    user32.GetWindowTextW(hwnd, buf, length + 1)
    return buf.value


def find_vlc_window():
    """Return the most likely visible top-level VLC window, or None."""
    titled = []
    untitled = []

    @ENUMWINDOWSPROC
    def enum_proc(hwnd, lparam):
        if not user32.IsWindowVisible(hwnd):
            return True

        pid = wintypes.DWORD()
        user32.GetWindowThreadProcessId(hwnd, ctypes.byref(pid))
        if pid.value and get_process_name(pid.value) == "vlc.exe":
            title = get_window_title(hwnd).strip()
            if title:
                titled.append(hwnd)
            else:
                untitled.append(hwnd)
        return True

    user32.EnumWindows(enum_proc, 0)

    if titled:
        # Usually the main VLC video/player window has a title.
        return titled[0]
    if untitled:
        return untitled[0]
    return None


def pause_and_minimize_vlc(hwnd):
    """
    Send the dedicated Windows MEDIA_PAUSE command (not play/pause toggle),
    then minimize VLC.
    """
    try:
        # Command ID is encoded in the high word of lParam.
        user32.SendMessageW(
            hwnd,
            WM_APPCOMMAND,
            hwnd,
            APPCOMMAND_MEDIA_PAUSE << 16,
        )
    finally:
        user32.ShowWindowAsync(hwnd, SW_MINIMIZE)


# ---------------- Global ESC hook ----------------
hook_handle = None
hook_thread_id = 0
hook_callback_ref = None
suppress_escape_until_keyup = False


def keyboard_hook_thread():
    global hook_handle, hook_thread_id, hook_callback_ref
    global suppress_escape_until_keyup

    hook_thread_id = kernel32.GetCurrentThreadId()

    @HOOKPROC
    def hook_proc(nCode, wParam, lParam):
        global suppress_escape_until_keyup

        if nCode == HC_ACTION:
            kb = ctypes.cast(
                lParam, ctypes.POINTER(KBDLLHOOKSTRUCT)
            ).contents

            if kb.vkCode == VK_ESCAPE:
                if wParam in (WM_KEYDOWN, WM_SYSKEYDOWN):
                    if suppress_escape_until_keyup:
                        return 1

                    hwnd = find_vlc_window()
                    if hwnd:
                        suppress_escape_until_keyup = True
                        pause_and_minimize_vlc(hwnd)
                        return 1

                elif wParam in (WM_KEYUP, WM_SYSKEYUP):
                    if suppress_escape_until_keyup:
                        suppress_escape_until_keyup = False
                        return 1

        return user32.CallNextHookEx(
            hook_handle, nCode, wParam, lParam
        )

    hook_callback_ref = hook_proc
    module = kernel32.GetModuleHandleW(None)
    hook_handle = user32.SetWindowsHookExW(
        WH_KEYBOARD_LL, hook_callback_ref, module, 0
    )

    if not hook_handle:
        return

    msg = wintypes.MSG()
    while user32.GetMessageW(ctypes.byref(msg), None, 0, 0) > 0:
        pass

    user32.UnhookWindowsHookEx(hook_handle)
    hook_handle = None


# ---------------- Tiny draggable smiley ----------------
root = tk.Tk()
root.overrideredirect(True)
root.attributes("-topmost", True)

TRANSPARENT = "#010203"
root.configure(bg=TRANSPARENT)

# Windows supports transparent color for a shaped tiny window.
try:
    root.wm_attributes("-transparentcolor", TRANSPARENT)
except tk.TclError:
    pass

screen_w = root.winfo_screenwidth()
root.geometry(f"20x20+{max(0, screen_w - 35)}+35")

canvas = tk.Canvas(
    root,
    width=20,
    height=20,
    bg=TRANSPARENT,
    highlightthickness=0,
    bd=0,
)
canvas.pack(fill="both", expand=True)

# Smiley face
canvas.create_oval(1, 1, 19, 19, fill="#FFD84D", outline="#202020", width=1)
canvas.create_oval(5, 6, 7, 8, fill="#202020", outline="")
canvas.create_oval(13, 6, 15, 8, fill="#202020", outline="")
canvas.create_arc(
    5, 6, 15, 15,
    start=205,
    extent=130,
    style=tk.ARC,
    outline="#202020",
    width=2,
)

_drag = {"x": 0, "y": 0}


def drag_start(event):
    _drag["x"] = event.x_root - root.winfo_x()
    _drag["y"] = event.y_root - root.winfo_y()


def dragging(event):
    x = event.x_root - _drag["x"]
    y = event.y_root - _drag["y"]

    # Keep at least part of it on-screen.
    x = max(-10, min(x, root.winfo_screenwidth() - 10))
    y = max(-10, min(y, root.winfo_screenheight() - 10))
    root.geometry(f"+{x}+{y}")


def quit_app(event=None):
    global hook_thread_id
    if hook_thread_id:
        user32.PostThreadMessageW(hook_thread_id, WM_QUIT, 0, 0)
    root.destroy()


canvas.bind("<ButtonPress-1>", drag_start)
canvas.bind("<B1-Motion>", dragging)
canvas.bind("<Button-3>", quit_app)

threading.Thread(target=keyboard_hook_thread, daemon=True).start()

try:
    root.mainloop()
finally:
    if hook_thread_id:
        user32.PostThreadMessageW(hook_thread_id, WM_QUIT, 0, 0)
