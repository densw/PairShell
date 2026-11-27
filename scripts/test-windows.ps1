param([switch]$CompileOnly,[string]$Filter='')
$ErrorActionPreference='Stop'
$repo=Split-Path $PSScriptRoot -Parent
. "$PSScriptRoot/build-tools.ps1"
$go=Find-PairShellTool 'go' @('go/bin/go.exe')
$output=Join-Path (Get-PairShellBuildWork) 'test-bin'
New-Item -ItemType Directory -Force -Path $output | Out-Null
$packages=& $go -C $repo list ./...
if($LASTEXITCODE -ne 0){throw 'Could not list test packages'}
foreach($package in $packages){
  $name=$package.Replace('pairshell/','').Replace('/','-')
  $executable=Join-Path $output ($name+'.test.exe')
  & $go -C $repo test -c -o $executable $package
  if($LASTEXITCODE -ne 0){throw "Compilation failed: $package"}
  if(-not $CompileOnly){
    $testArgs=@('-test.timeout=90s')
    if($Filter){$testArgs+=('-test.run='+$Filter)}
    $packageDir=& $go -C $repo list -f '{{.Dir}}' $package
    Push-Location -LiteralPath $packageDir
    try {
      & $executable @testArgs
      if($LASTEXITCODE -ne 0){throw "Test failed: $package"}
    } finally {Pop-Location}
  }
}
