param([string]$ReportPath='')
$ErrorActionPreference='Stop'
try {
$repo=Split-Path $PSScriptRoot -Parent
. "$PSScriptRoot/build-tools.ps1"
$work=Get-PairShellBuildWork
if(-not $ReportPath){$ReportPath=Join-Path $work 'firewall-allow-report.json'}
$admin=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if(-not $admin.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){throw 'Windows firewall changes require administrator rights'}
$paths=@(
  (Join-Path $repo 'bin/pairshell-windows-amd64.exe'),
  (Join-Path $repo 'bin/pairshelld-windows-amd64.exe')
)
$testBin=Join-Path $work 'test-bin'
if(Test-Path -LiteralPath $testBin){
  $paths+=@(Get-ChildItem -LiteralPath $testBin -Filter '*.test.exe' -File | ForEach-Object FullName)
}
$paths=@($paths | Where-Object {Test-Path -LiteralPath $_ -PathType Leaf} | ForEach-Object {[IO.Path]::GetFullPath($_)} | Sort-Object -Unique)
$results=@()
foreach($program in $paths){
  $hasher=[Security.Cryptography.SHA256]::Create()
  try {$hash=$hasher.ComputeHash([Text.Encoding]::UTF8.GetBytes($program.ToLowerInvariant()))} finally {$hasher.Dispose()}
  $key=([BitConverter]::ToString($hash).Replace('-','')).Substring(0,20)
  foreach($protocol in @('TCP','UDP')){
    $name="PairShell-LAN-$key-$protocol"
    $existing=Get-NetFirewallRule -Name $name -ErrorAction SilentlyContinue
    if($existing){$existing | Remove-NetFirewallRule}
    New-NetFirewallRule -Name $name -DisplayName "PairShell LAN test $protocol - $(Split-Path $program -Leaf)" -Group 'PairShell LAN Tests' -Direction Inbound -Action Allow -Enabled True -Profile Private -Program $program -Protocol $protocol -RemoteAddress '192.168.31.0/24' | Out-Null
  }
  # Explicit per-program block rules override allows; retire only blocks for
  # these exact reviewed files. Default public-network blocking stays intact.
  $filters=@(Get-NetFirewallApplicationFilter -Program $program -ErrorAction SilentlyContinue)
  foreach($filter in $filters){
    Get-NetFirewallRule -AssociatedNetFirewallApplicationFilter $filter | Where-Object {$_.Action -eq 'Block' -and $_.Enabled -eq 'True'} | Disable-NetFirewallRule
  }
  $results+=@{program=$program;sha256=(Get-FileHash -LiteralPath $program -Algorithm SHA256).Hash;protocols=@('TCP','UDP');profile='Private';remote='192.168.31.0/24'}
}
$report=@{status='OK';program_count=$paths.Count;rules=2*$paths.Count;programs=$results}
if($ReportPath){$report | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $ReportPath -Encoding utf8}
$report | ConvertTo-Json -Depth 5
} catch {
  if($ReportPath){@{status='FAILED';message=$_.Exception.Message} | ConvertTo-Json | Set-Content -LiteralPath $ReportPath -Encoding utf8}
  throw
}
