@echo off
setlocal
title Command Preflight Setup

set "source=%~dp0command-preflight.exe"
set "target=%LOCALAPPDATA%\CommandPreflight"
if not exist "%source%" (
  echo command-preflight.exe was not found beside this file.
  echo Extract the complete release archive, then run INSTALL.cmd again.
  pause
  exit /b 1
)

if not exist "%target%" mkdir "%target%"
copy /Y "%source%" "%target%\command-preflight.exe" >nul
if errorlevel 1 (
  echo Could not copy the executable to "%target%".
  pause
  exit /b 1
)

powershell -NoProfile -ExecutionPolicy Bypass -Command "$target = Join-Path $env:LOCALAPPDATA 'CommandPreflight'; $userPath = [Environment]::GetEnvironmentVariable('Path', 'User'); $parts = @($userPath -split ';' | Where-Object { $_ }); if ($parts -notcontains $target) { [Environment]::SetEnvironmentVariable('Path', (($parts + $target) -join ';'), 'User') }"
set "PATH=%target%;%PATH%"

echo Installing the bundled Codex and Claude Code Skill files...
"%target%\command-preflight.exe" install-skill --target both
if errorlevel 1 goto failed

echo.
choice /C YN /N /M "Enable optional read-only community knowledge lookup? [Y/N] "
if errorlevel 2 goto offline
set "COMMAND_PREFLIGHT_KNOWLEDGE_URL=https://preflight.52131415.xyz"
echo Community lookup enabled for this MCP registration.
goto setup

:offline
set "COMMAND_PREFLIGHT_KNOWLEDGE_URL="
echo Knowledge lookup stays offline.

:setup
echo Registering MCP integrations for installed clients...
"%target%\command-preflight.exe" setup --client both --apply
if errorlevel 1 echo MCP registration needs manual review; the binary is still installed.
echo.
echo Installed command-preflight in "%target%".
echo Open a new terminal before using it.
pause
exit /b 0

:failed
echo Installation failed. The executable may be blocked by Windows security policy.
pause
exit /b 1
