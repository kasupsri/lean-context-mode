@echo off
setlocal

set "BIN=%~dp0..\lean-context-mode.exe"
if not exist "%BIN%" (
  echo lean-context-mode.exe not found. Build it first:
  echo   go build -o lean-context-mode.exe .\cmd\lean-context-mode
  exit /b 1
)

if "%~1"=="" (
  if not defined LCM_ROOT (
    set "LCM_ROOT=%CD%"
  )
  "%BIN%" serve
) else (
  "%BIN%" serve --root "%~1"
)
