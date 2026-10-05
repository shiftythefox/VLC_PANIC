@echo off
setlocal
cd /d "%~dp0"
if not exist release mkdir release
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags "-H=windowsgui -s -w" -o release\VLC_ESC_Smiley_v2.exe .\src
if errorlevel 1 (
  echo.
  echo BUILD FAILED
  pause
  exit /b 1
)
echo.
echo Built: release\VLC_ESC_Smiley_v2.exe
pause
