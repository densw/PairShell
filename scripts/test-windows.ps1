param([switch]$CompileOnly,[string]$Filter='')
$ErrorActionPreference='Stop'
$repo=Split-Path $PSScriptRoot -Parent
$go=Join-Path $repo '.tools/go/bin/go.exe'
if(-not(Test-Path -LiteralPath $go)){$go='go'}
$output=Join-Path $repo '.tools/test-bin'
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
