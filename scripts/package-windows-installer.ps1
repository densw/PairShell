param([string]$Version='1.1.3',[ValidateSet('Customer','Merchant','Both')][string]$Role='Both')
$ErrorActionPreference='Stop'
if($Version -notmatch '^\d+\.\d+\.\d+$'){throw 'Invalid version'}
$repo=Split-Path $PSScriptRoot -Parent
$source=Join-Path $repo "bin/PairShell-$Version-Windows-x64"
$compiler=Get-ChildItem -LiteralPath (Join-Path $repo '.tools/nsis') -Filter makensis.exe -Recurse | Select-Object -First 1 -ExpandProperty FullName
if(-not $compiler){throw 'NSIS portable compiler is required'}
$roles=if($Role -eq 'Both'){@('Customer','Merchant')}else{@($Role)}
foreach($currentRole in $roles){
 $label=if($currentRole -eq 'Customer'){'客户端'}else{'商家端'}
 $stage=Join-Path $repo ('.tools/installer-'+$currentRole+'-'+[guid]::NewGuid().ToString('N'))
 $package=Join-Path $stage 'payload'
 New-Item -ItemType Directory -Force -Path $package | Out-Null
 $roleRuntime=Join-Path $package 'runtime'
 New-Item -ItemType Directory -Path $roleRuntime | Out-Null
 Get-ChildItem -LiteralPath (Join-Path $source 'runtime') -Force | Where-Object Name -ne '.pairshell-portable' | ForEach-Object {Copy-Item -LiteralPath $_.FullName -Destination $roleRuntime -Recurse}
 $launcherCompiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
 $defaultRole=if($currentRole -eq 'Customer'){'CUSTOMER_DEFAULT'}else{'MERCHANT_DEFAULT'}
 & $launcherCompiler /nologo /target:winexe /reference:System.Windows.Forms.dll ("/define:"+$defaultRole) ("/win32icon:"+(Join-Path $repo 'assets/brand/pairshell.ico')) ("/out:"+(Join-Path $package 'PairShell.exe')) (Join-Path $repo 'native/windows/Launcher.cs')
 if($LASTEXITCODE -ne 0){throw 'Default-role launcher compilation failed'}
 $action=if($currentRole -eq 'Customer'){'接受协助'}else{'发起协助'}
 @("PairShell $Version",("默认启动："+$action+'。应用内可切换发起协助／接受协助。'),'默认使用官方通道 https://relay.pairshell.top；可在设置中填写自定义服务器。','保留 runtime 子目录；配置与历史保存在当前用户目录，卸载时保留。','安装目录内运行 Uninstall.exe，或从 Windows 应用列表卸载 PairShell。') | Set-Content -LiteralPath (Join-Path $package '使用说明.txt') -Encoding utf8
 $deletes=[Collections.Generic.List[string]]::new()
 foreach($file in Get-ChildItem -LiteralPath $package -Recurse -File){
  $relative=[IO.Path]::GetRelativePath($package,$file.FullName)
  if($relative.Contains('..') -or $relative.Contains('$') -or $relative.Contains('"')){throw 'Unsafe package filename'}
  $deletes.Add('  Delete "$INSTDIR\'+$relative+'"')
 }
 foreach($dir in Get-ChildItem -LiteralPath $package -Recurse -Directory | Sort-Object {$_.FullName.Length} -Descending){
  $relative=[IO.Path]::GetRelativePath($package,$dir.FullName)
  $deletes.Add('  RMDir "$INSTDIR\'+$relative+'"')
 }
 $deletes | Set-Content -LiteralPath (Join-Path $stage 'uninstall-files.nsh') -Encoding utf8BOM
 $output=Join-Path $repo "bin/PairShell-$Version-Windows-x64-$currentRole-Setup.exe"
 $script=[IO.File]::ReadAllText((Join-Path $repo 'native/windows/installer.nsi'))
 $script=$script.Replace('@VERSION@',$Version).Replace('@OUTPUT@',$output).Replace('@ICON@',(Join-Path $repo 'assets/brand/pairshell.ico')).Replace('@PACKAGE@',$package).Replace('@ROLE@',$currentRole).Replace('@LABEL@',$label)
 $script | Set-Content -LiteralPath (Join-Path $stage 'installer.nsi') -Encoding utf8BOM
 & $compiler /V2 (Join-Path $stage 'installer.nsi')
 if($LASTEXITCODE -ne 0){throw "Installer build failed: $currentRole"}
 $distribution=Join-Path $repo "bin/Windows-$currentRole"
 New-Item -ItemType Directory -Force -Path $distribution | Out-Null
 Copy-Item -LiteralPath $output -Destination (Join-Path $distribution 'PairShell-Setup.exe') -Force
 Get-FileHash -LiteralPath $output -Algorithm SHA256
}
