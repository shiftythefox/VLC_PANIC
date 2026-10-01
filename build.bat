@echo off
setlocal
cd /d "%~dp0"

where go >nul 2>nul
if errorlevel 1 (
  echo [HIBA] A Go nincs telepitve vagy nincs a PATH-ban.
  echo Letoltes: https://go.dev/dl/
  pause
  exit /b 1
)

if not exist release mkdir release

echo VLC_ESC_Smiley.exe epitese...
set GOOS=windows
set GOARCH=amd64
go build -buildvcs=false -trimpath -ldflags="-s -w -H=windowsgui" -o release\VLC_ESC_Smiley.exe .\src

if errorlevel 1 (
  echo [HIBA] A build sikertelen.
  pause
  exit /b 1
)

echo.
echo Kesz: release\VLC_ESC_Smiley.exe
pause
