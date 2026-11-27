param([string]$Version='1.1.3')
$ErrorActionPreference = 'Stop'
if($Version -notmatch '^\d+\.\d+\.\d+$'){throw 'Version must use major.minor.patch'}
$repo = Split-Path $PSScriptRoot -Parent
. "$PSScriptRoot/build-tools.ps1"
$go = Find-PairShellTool 'go' @('go/bin/go.exe')
$dotnet = Find-PairShellTool 'dotnet' @('dotnet10/dotnet.exe')
if (-not ((& $dotnet --list-sdks) -match '^10\.')) {
  $dotnet = Find-PairShellTool 'dotnet' @('dotnet10/dotnet.exe') -CacheOnly
}
$oldOS=$env:GOOS; $oldArch=$env:GOARCH
$stage=$null
try {
  $env:GOOS='windows'; $env:GOARCH='amd64'
  $output = Join-Path $repo "bin/PairShell-$Version-Windows-x64"
  if(Test-Path -LiteralPath $output){throw "Release directory already exists; choose a fresh version or archive it first: $output"}
  $stage = Join-Path (Get-PairShellBuildWork) ('windows-publish-'+[guid]::NewGuid().ToString('N'))
  $publish = Join-Path $stage 'publish'
  $intermediate = (Join-Path $stage 'obj') + [IO.Path]::DirectorySeparatorChar
  $runtime = Join-Path $output 'runtime'
  New-Item -ItemType Directory -Force -Path $runtime | Out-Null
  & $dotnet publish "$repo/native/windows/PairShell.Customer.csproj" -c Release -r win-x64 --self-contained true -p:Version=$Version --artifacts-path $stage -p:IncludeProjectNameInArtifactsPaths=false -o $publish
  if ($LASTEXITCODE -ne 0) { throw 'Windows shell build failed' }
  Copy-Item -LiteralPath (Join-Path $publish 'PairShell.exe') -Destination $runtime
  $launcherCompiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
  if(-not(Test-Path -LiteralPath $launcherCompiler)){throw '.NET Framework launcher compiler is required'}
  $launcherSource=Join-Path $repo 'native\windows\Launcher.cs'
  $launcherOutput=Join-Path $output 'PairShell.Customer.exe'
  $launcherIcon=Join-Path $repo 'assets\brand\pairshell.ico'
  & $launcherCompiler /nologo /target:winexe /reference:System.Windows.Forms.dll "/win32icon:$launcherIcon" "/out:$launcherOutput" $launcherSource
  if($LASTEXITCODE -ne 0){throw 'Role launcher build failed'}
  Copy-Item -LiteralPath "$output/PairShell.Customer.exe" -Destination "$output/PairShell.Merchant.exe"
  Get-ChildItem -LiteralPath $publish | Where-Object Name -ne 'PairShell.exe' | ForEach-Object {Copy-Item -LiteralPath $_.FullName -Destination $runtime -Recurse -Force}
  # Give WebView2 an explicit native loader path instead of depending on bundle extraction lookup.
  $assets=Get-Content (Join-Path $intermediate 'project.assets.json') -Raw | ConvertFrom-Json
  $webview=($assets.libraries.PSObject.Properties | Where-Object Name -like 'Microsoft.Web.WebView2/*' | Select-Object -First 1).Value.path
  $loader=$null
  foreach($folder in $assets.packageFolders.PSObject.Properties.Name){
    $candidate=Join-Path $folder ($webview+'/runtimes/win-x64/native/WebView2Loader.dll')
    if(Test-Path -LiteralPath $candidate){$loader=$candidate;break}
  }
  if(-not $loader){throw 'WebView2 x64 native loader was not restored'}
  Copy-Item -LiteralPath $loader -Destination $runtime
  & $go -C $repo build -trimpath -o "$runtime/PairShellBackend.exe" ./cmd/pairshell
  if ($LASTEXITCODE -ne 0) { throw 'Windows backend build failed' }
  $legal=Join-Path $runtime 'licenses'
  New-Item -ItemType Directory -Force -Path $legal | Out-Null
  Copy-Item "$repo/docs/THIRD_PARTY_NOTICES.md","$repo/docs/WEBVIEW2-LICENSE.txt","$repo/docs/WEBVIEW2-NOTICE.txt" $legal
  @("PairShell $Version",'PairShell.Customer.exe：默认接受协助；PairShell.Merchant.exe：默认发起协助。应用内仍可互相切换。','两个入口共用 runtime 中的核心程序。客户核对校验码后，只需允许一次完整协助。','默认使用官方通道 https://relay.pairshell.top；自定义服务器可在设置中配置。配置和历史保存在当前用户目录。','安装版与便携版均提供 Uninstall.exe；请先关闭该目录中的 PairShell。卸载保留配置、历史和自行放入的其他文件。','请保留 runtime 子目录。若缺少 WebView2 Runtime，请安装微软官方运行时。') | Set-Content "$output/使用说明.txt" -Encoding utf8
  & (Join-Path $PSScriptRoot 'package-windows-portable-uninstaller.ps1') -Version $Version
  $rootFiles=@(Get-ChildItem -LiteralPath $output -File)
  if($rootFiles.Count -ne 4 -or -not(Test-Path "$output/Uninstall.exe") -or -not(Test-Path "$output/PairShell.Customer.exe") -or -not(Test-Path "$output/PairShell.Merchant.exe") -or -not(Test-Path "$runtime/PairShell.exe") -or -not(Test-Path "$runtime/PairShellBackend.exe")){throw 'Invalid clean release layout'}
  Compress-Archive "$output/*" "$repo/bin/PairShell-$Version-Windows-x64.zip" -Force
} finally {
  $env:GOOS=$oldOS; $env:GOARCH=$oldArch
  Remove-PairShellBuildStage $stage
}
