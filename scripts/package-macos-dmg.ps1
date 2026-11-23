param([string]$Version='1.1.2')
$ErrorActionPreference='Stop'
if($Version -notmatch '^\d+\.\d+\.\d+$'){throw 'Invalid version'}
$repo=Split-Path $PSScriptRoot -Parent
$go=Join-Path $repo '.tools/go/bin/go.exe'
$module=Join-Path $repo '.tools/dmg-builder'
New-Item -ItemType Directory -Force -Path $module | Out-Null
if(-not(Test-Path -LiteralPath (Join-Path $module 'go.mod'))){
 & $go -C $module mod init pairshell-dmg-builder
 if($LASTEXITCODE -ne 0){throw 'Build module init failed'}
}
# Build-only dependency; never added to the application's go.mod.
& $go -C $module get github.com/leaanthony/dmg@v0.0.0-20260731075144-3d3581c3dfa3
if($LASTEXITCODE -ne 0){throw 'DMG tool dependency restore failed'}
Copy-Item -LiteralPath (Join-Path $repo 'scripts/dmg-builder.go') -Destination (Join-Path $module 'main.go') -Force
$tool=Join-Path $repo '.tools/pairshell-dmg-builder.exe'
& $go -C $module build -o $tool main.go
if($LASTEXITCODE -ne 0){throw 'DMG tool build failed'}
python (Join-Path $repo 'scripts/package-macos-universal.py') --version $Version
if($LASTEXITCODE -ne 0){throw 'Universal App assembly failed'}
$zip=Join-Path $repo "bin/PairShell-$Version-macOS-universal.zip"
$dmg=Join-Path $repo "bin/PairShell-$Version-macOS-universal.dmg"
$temporary=Join-Path $repo ('.tools/PairShell-'+$Version+'-'+[guid]::NewGuid().ToString('N')+'.dmg')
& $tool $zip $temporary
if($LASTEXITCODE -ne 0){throw 'DMG creation failed'}
Move-Item -LiteralPath $temporary -Destination $dmg -Force
Get-FileHash -LiteralPath $dmg -Algorithm SHA256
