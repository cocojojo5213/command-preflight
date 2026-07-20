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
    $executable = Join-Path $installDirectory 'command-preflight.exe'
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $pathParts = @($userPath -split ';' | Where-Object { $_ })
    if ($pathParts -notcontains $installDirectory) {
        [Environment]::SetEnvironmentVariable('Path', (($pathParts + $installDirectory) -join ';'), 'User')
    }
    $env:Path = "$installDirectory;$env:Path"
    & $executable install-skill --target both
    & $executable setup --client both --apply
    if ($LASTEXITCODE -ne 0) {
        Write-Warning 'The binary was installed, but automatic MCP registration needs to be rerun after checking the client CLI.'
    }
    Write-Host "Installed $installDirectory\command-preflight.exe"
    if ($env:COMMAND_PREFLIGHT_KNOWLEDGE_URL) {
        Write-Host "Opt-in knowledge lookup configured for: $env:COMMAND_PREFLIGHT_KNOWLEDGE_URL"
    }
    else {
        Write-Host 'Knowledge lookup remains offline (set COMMAND_PREFLIGHT_KNOWLEDGE_URL to opt in).'
    }
    if ($env:COMMAND_PREFLIGHT_REPORTING -match '^(?i:on|true|yes|1)$') {
        $reportUrl = if ($env:COMMAND_PREFLIGHT_REPORT_URL) { $env:COMMAND_PREFLIGHT_REPORT_URL } else { $env:COMMAND_PREFLIGHT_KNOWLEDGE_URL }
        Write-Host "Opt-in moderated reporting configured for: $reportUrl"
    }
    else {
        Write-Host 'Community reporting remains disabled (set COMMAND_PREFLIGHT_REPORTING=on to opt in).'
    }
    Write-Host 'Open a new terminal before using command-preflight.'
}
finally {
    Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
}
