@echo off
rem Launcher for Windows: put helios7.exe next to this file and double-click.
setlocal
cd /d "%~dp0"

if not exist "helios7.exe" (
  echo helios7.exe not found in this folder.
  echo Download it from https://github.com/ShRekiSss/Ob/releases
  pause
  exit /b 1
)

rem If port 8080 is busy, uncomment the next line and change the port.
rem set ADDR=127.0.0.1:9090

echo Starting Helios-7... The browser will open automatically.
echo Close this window to stop the server.
echo.
helios7.exe
pause
