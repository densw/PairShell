$ErrorActionPreference = 'Stop'
$previousOS = $env:GOOS; $previousArch = $env:GOARCH; $previousCGO = $env:CGO_ENABLED
try {
$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) { $go = Get-Item "$PSScriptRoot/../.tools/go/bin/go.exe" }
$targets = @('windows/amd64','linux/amd64','linux/arm64','darwin/amd64','darwin/arm64')
New-Item -ItemType Directory -Force "$PSScriptRoot/../bin" | Out-Null
foreach ($target in $targets) {
  $osName, $archName = $target.Split('/')
  $env:GOOS = $osName; $env:GOARCH = $archName; $env:CGO_ENABLED = '0'
  foreach ($program in @('pairshell','pairshelld')) {
    $suffix = if ($osName -eq 'windows') { '.exe' } else { '' }
    & $go -C "$PSScriptRoot/.." build -trimpath -o "bin/$program-$osName-$archName$suffix" "./cmd/$program"
    if ($LASTEXITCODE -ne 0) { throw "Build failed: $target $program" }
  }
}
Get-ChildItem "$PSScriptRoot/../bin" -File | Where-Object Name -ne 'SHA256SUMS' | Get-FileHash -Algorithm SHA256 | ForEach-Object { "$($_.Hash.ToLower())  $(Split-Path $_.Path -Leaf)" } | Set-Content "$PSScriptRoot/../bin/SHA256SUMS"
} finally {
  $env:GOOS = $previousOS; $env:GOARCH = $previousArch; $env:CGO_ENABLED = $previousCGO
}
