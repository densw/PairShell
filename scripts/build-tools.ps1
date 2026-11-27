# Build tools live outside the source checkout. Prefer installed commands.
function Get-PairShellBuildTools {
  if ($env:PAIRSHELL_BUILD_TOOLS) { return $env:PAIRSHELL_BUILD_TOOLS }
  return Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'PairShell/BuildTools'
}

function Find-PairShellTool([string]$Name, [string[]]$RelativePaths, [switch]$CacheOnly) {
  $command = if (-not $CacheOnly) { Get-Command $Name -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1 }
  if ($command) { return $command.Source }
  $defaultCache = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'PairShell/BuildTools'
  foreach ($cache in @($env:PAIRSHELL_BUILD_TOOLS, $defaultCache) | Where-Object { $_ } | Select-Object -Unique) {
    foreach ($relative in $RelativePaths) {
      $candidate = Join-Path $cache $relative
      if (Test-Path -LiteralPath $candidate -PathType Leaf) { return $candidate }
    }
  }
  throw "Required build tool '$Name' was not found on PATH or in the PairShell build tools cache"
}

function Get-PairShellBuildWork {
  $work = Join-Path (Get-PairShellBuildTools) 'work'
  New-Item -ItemType Directory -Force -Path $work | Out-Null
  return $work
}

function Remove-PairShellBuildStage([string]$Stage) {
  if (-not $Stage -or -not (Test-Path -LiteralPath $Stage)) { return }
  $work = [IO.Path]::GetFullPath((Get-PairShellBuildWork)).TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
  $target = [IO.Path]::GetFullPath($Stage)
  if (-not $target.StartsWith($work, [StringComparison]::OrdinalIgnoreCase)) { throw 'Build stage is outside the work directory' }
  Remove-Item -LiteralPath $target -Recurse -Force
}
