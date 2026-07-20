$ErrorActionPreference = 'Stop'

$repository = if ($env:COMMAND_PREFLIGHT_REPO) { $env:COMMAND_PREFLIGHT_REPO } else { 'cocojojo5213/command-preflight' }
$architecture = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}
$asset = "command-preflight_windows_$architecture.zip"
$temporaryDirectory = Join-Path ([System.IO.Path]::GetTempPath()) ("command-preflight-" + [guid]::NewGuid().ToString('N'))
$installDirectory = if ($env:COMMAND_PREFLIGHT_PREFIX) { $env:COMMAND_PREFLIGHT_PREFIX } else { Join-Path $env:LOCALAPPDATA 'CommandPreflight' }

New-Item -ItemType Directory -Path $temporaryDirectory -Force | Out-Null
New-Item -ItemType Directory -Path $installDirectory -Force | Out-Null
try {
    $archive = Join-Path $temporaryDirectory $asset
    Invoke-WebRequest -Uri "https://github.com/$repository/releases/latest/download/$asset" -OutFile $archive
    Expand-Archive -LiteralPath $archive -DestinationPath $temporaryDirectory -Force
    Copy-Item -LiteralPath (Join-Path $temporaryDirectory 'command-preflight.exe') -Destination (Join-Path $installDirectory 'command-preflight.exe') -Force
    & (Join-Path $installDirectory 'command-preflight.exe') install-skill --target both
    Write-Host "Installed $installDirectory\command-preflight.exe"
    Write-Host 'Register MCP when ready:'
    Write-Host "  & '$installDirectory\command-preflight.exe' setup --client both --apply"
    Write-Host 'Add the install directory to PATH if command-preflight is not recognized.'
}
finally {
    Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
}
