# Run on a disposable Windows CI runner after windows/build.ps1.
param([string]$Packages = (Join-Path (Split-Path -Parent $PSScriptRoot) 'dist'))
$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$arch = if ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }
$name = "ghosts_dev_windows_$arch.zip"
$fixtureArchivePath = Join-Path $Packages $name
$installer = Join-Path $repo 'packaging/install.ps1'
$installDir = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'Programs\Ghosts'
$fixtureTag = 'v1.0.0'
$asset = $name.Replace('_dev_', '_1.0.0_')
$base = "https://github.com/xamjiang/ghosts/releases/download/$fixtureTag"
$hash = (Get-FileHash -LiteralPath $fixtureArchivePath).Hash
function Invoke-RestMethod($Uri) {
    if ($Uri -ne 'https://api.github.com/repos/xamjiang/ghosts/releases/latest') { throw "Unexpected API request: $Uri" }
    return @{ tag_name = $fixtureTag; assets = @([pscustomobject]@{name=$asset}) }
}
function Invoke-WebRequest([string]$Uri, [switch]$UseBasicParsing, [string]$OutFile) {
    if ($Uri -eq "$base/checksums.txt") { return @{ Content = "$hash  $asset" } }
    if ($Uri -ne "$base/$asset" -or !$OutFile) { throw "Unexpected asset request: $Uri" }
    Copy-Item -LiteralPath $fixtureArchivePath -Destination $OutFile
}

# Exercise the same string invocation used by the online one-liner.
$scriptText = Get-Content -LiteralPath $installer -Raw -Encoding UTF8
Invoke-Expression $scriptText
$zipInstaller = Join-Path $Packages "windows_$arch/install.cmd"
& $zipInstaller
if ($LASTEXITCODE -ne 0) { throw 'Offline reinstall failed' }

foreach ($command in 'ghost','ghosts') {
    $expected = Join-Path $installDir "$command.exe"
    if ((Get-Command $command -CommandType Application | Select-Object -First 1).Source -ne $expected) { throw "Unexpected $command path" }
    $version = cmd.exe /d /c "$command -version"
    if ($LASTEXITCODE -ne 0 -or $version -ne 'ghosts dev') { throw "$command did not run" }
}
$userPath = [Environment]::GetEnvironmentVariable('Path','User')
if (@($userPath.Split(';') | Where-Object { $_.TrimEnd('\') -eq $installDir }).Count -ne 1) { throw 'Duplicate or missing PATH entry' }

$hash = '0' * 64
try { Invoke-Expression $scriptText; throw 'Installer accepted an incorrect checksum' }
catch { if ($_.Exception.Message -notlike '*failed SHA-256 verification*') { throw } }
if ((Get-FileHash -LiteralPath (Join-Path $installDir 'ghost.exe')).Hash -ne (Get-FileHash -LiteralPath (Join-Path $Packages "windows_$arch/ghosts.exe")).Hash) {
    throw 'A failed reinstall changed the installed CLI'
}
Write-Output 'Windows online fixture, offline reinstall, PATH, aliases and checksum rejection passed.'
