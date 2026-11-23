param([Parameter(Mandatory)][string]$Executable,[Parameter(Mandatory)][ValidateSet('customer','merchant')][string]$ExpectedRole)
$ErrorActionPreference='Stop'
# Inspect only the compiled pure default-selector method; never start the UI.
$assembly=[Reflection.Assembly]::Load([IO.File]::ReadAllBytes($Executable))
$method=$assembly.GetType('RoleLauncher').GetMethod('DefaultFlags',[Reflection.BindingFlags]'Static,NonPublic')
if(-not $method){throw 'Default-role selector missing'}
$actual=$method.Invoke($null,@())
if($actual -ne ('--'+$ExpectedRole)){throw "Wrong default role: $actual"}
Write-Output "Compiled default role: $ExpectedRole PASS"
