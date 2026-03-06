@echo off
setlocal

if "%~1"=="" (
  echo Usage: %~nx0 "C:\path\to\workspace"
  exit /b 1
)

set "ROOT=%~1"
setx LCM_ROOT "%ROOT%" >nul
if errorlevel 1 (
  echo Failed to persist LCM_ROOT.
  exit /b 1
)

echo LCM_ROOT saved as: %ROOT%
echo Open a new terminal for the updated environment value.
