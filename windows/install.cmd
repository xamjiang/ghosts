@echo off
setlocal
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1" -PackageDirectory "%~dp0."
if errorlevel 1 (
  echo Installation failed. If Ghosts is running, quit it and try again.
  pause
  exit /b 1
)
