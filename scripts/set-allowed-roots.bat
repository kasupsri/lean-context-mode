@echo off
setlocal

if "%~1"=="" (
  echo Usage: %~nx0 "C:\path\one;C:\path\two"
  exit /b 1
)

set "ROOTS=%~1"
setx LCM_ALLOWED_ROOTS "%ROOTS%" >nul
if errorlevel 1 (
  echo Failed to persist LCM_ALLOWED_ROOTS.
  exit /b 1
)

echo LCM_ALLOWED_ROOTS saved as: %ROOTS%
echo Open a new terminal for the updated environment value.
