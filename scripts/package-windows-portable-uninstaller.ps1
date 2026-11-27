param([string]$Version='1.1.3')
$ErrorActionPreference='Stop'
if($Version -notmatch '^\d+\.\d+\.\d+$'){throw 'Invalid version'}
$repo=Split-Path $PSScriptRoot -Parent
$package=Join-Path $repo "bin/PairShell-$Version-Windows-x64"
foreach($name in @('PairShell.Customer.exe','PairShell.Merchant.exe','runtime/PairShell.exe','runtime/PairShellBackend.exe')){
 if(-not(Test-Path -LiteralPath (Join-Path $package $name))){throw "Missing portable component: $name"}
}
$compiler=Get-ChildItem -LiteralPath (Join-Path $repo '.tools/nsis') -Filter makensis.exe -Recurse | Select-Object -First 1 -ExpandProperty FullName
$seven=Join-Path $repo '.tools/release-0.10.1/mac/7zip-full/7z.exe'
if(-not $compiler -or -not(Test-Path -LiteralPath $seven)){throw 'NSIS and 7-Zip build tools are required'}
$stage=Join-Path $repo ('.tools/portable-uninstaller-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $stage | Out-Null
[IO.File]::WriteAllText((Join-Path $package 'runtime/.pairshell-portable'),'PairShell portable',[Text.UTF8Encoding]::new($false))
$deletes=[Collections.Generic.List[string]]::new()
foreach($file in Get-ChildItem -LiteralPath $package -Recurse -Force -File | Where-Object Name -ne 'Uninstall.exe'){
 $relative=[IO.Path]::GetRelativePath($package,$file.FullName)
 if($relative.Contains('..') -or $relative.Contains('$') -or $relative.Contains('"')){throw 'Unsafe package filename'}
 $deletes.Add(' Delete "$INSTDIR\'+$relative+'"')
}
foreach($dir in Get-ChildItem -LiteralPath $package -Recurse -Directory | Sort-Object {$_.FullName.Length} -Descending){
 $deletes.Add(' RMDir "$INSTDIR\'+[IO.Path]::GetRelativePath($package,$dir.FullName)+'"')
}
$deletes | Set-Content -LiteralPath (Join-Path $stage 'portable-files.nsh') -Encoding utf8BOM
$builder=Join-Path $stage 'build-only.exe'
$script=[IO.File]::ReadAllText((Join-Path $repo 'native/windows/portable-uninstall.nsi')).Replace('@BUILDER@',$builder).Replace('@ICON@',(Join-Path $repo 'assets/brand/pairshell.ico'))
$script | Set-Content -LiteralPath (Join-Path $stage 'portable-uninstall.nsi') -Encoding utf8BOM
& $compiler /V2 (Join-Path $stage 'portable-uninstall.nsi')
if($LASTEXITCODE -ne 0){throw 'Portable uninstaller compilation failed'}
& $seven e $builder ("-o"+$stage) Uninstall.exe -y | Out-Null
if($LASTEXITCODE -ne 0){throw 'Uninstaller extraction failed'}
Copy-Item -LiteralPath (Join-Path $stage 'Uninstall.exe') -Destination (Join-Path $package 'Uninstall.exe') -Force
Get-FileHash -LiteralPath (Join-Path $package 'Uninstall.exe') -Algorithm SHA256
