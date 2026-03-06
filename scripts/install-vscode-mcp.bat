@echo off
setlocal EnableExtensions EnableDelayedExpansion

set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..") do set "REPO_ROOT=%%~fI"
set "SERVE_BAT=%REPO_ROOT%\scripts\serve.bat"

if not exist "%SERVE_BAT%" (
  echo Could not find serve script: %SERVE_BAT%
  exit /b 1
)

if not "%~1"=="" (
  set "TARGET_ROOT=%~1"
  setx LCM_ROOT "%TARGET_ROOT%" >nul
  if errorlevel 1 (
    echo Failed to persist LCM_ROOT.
    exit /b 1
  )
  echo LCM_ROOT saved as: %TARGET_ROOT%
)

set "CURSOR_DIR=%USERPROFILE%\.cursor"
set "CURSOR_FILE=%CURSOR_DIR%\mcp.json"
if not exist "%CURSOR_DIR%" mkdir "%CURSOR_DIR%"
if exist "%CURSOR_FILE%" copy /Y "%CURSOR_FILE%" "%CURSOR_FILE%.bak" >nul

set "SERVE_JSON=%SERVE_BAT:\=/%"
(
  echo {
  echo   "mcpServers": {
  echo     "lean-context-mode": {
  echo       "command": "cmd",
  echo       "args": [
  echo         "/c",
  echo         "%SERVE_JSON%"
  echo       ]
  echo     }
  echo   }
  echo }
) > "%CURSOR_FILE%"

set "CODEX_DIR=%USERPROFILE%\.codex"
set "CODEX_FILE=%CODEX_DIR%\config.toml"
if not exist "%CODEX_DIR%" mkdir "%CODEX_DIR%"
if not exist "%CODEX_FILE%" type nul > "%CODEX_FILE%"

findstr /C:"[mcp_servers.lean-context-mode]" "%CODEX_FILE%" >nul
if errorlevel 1 (
  >> "%CODEX_FILE%" echo.
  >> "%CODEX_FILE%" echo [mcp_servers.lean-context-mode]
  >> "%CODEX_FILE%" echo command = "cmd"
  >> "%CODEX_FILE%" echo args = ["/c", '%SERVE_BAT%']
  echo Added lean-context-mode to %CODEX_FILE%
) else (
  echo lean-context-mode already exists in %CODEX_FILE%
)

echo Cursor MCP config written to %CURSOR_FILE%
echo Restart Cursor / VS Code before using the MCP tools.
echo If you used setx, open a new terminal so LCM_ROOT is available.

exit /b 0
