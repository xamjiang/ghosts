[CmdletBinding()]
param(
    [ValidateSet('amd64', 'arm64')]
    [string]$Arch = $(if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }),
    [ValidatePattern('^[a-zA-Z0-9][a-zA-Z0-9._-]*$')]
    [string]$Version = 'dev',
    [string]$OutputDirectory = (Join-Path (Split-Path -Parent $PSScriptRoot) 'dist')
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$dest = Join-Path $OutputDirectory "windows_$Arch"
$compiler = Join-Path $env:WINDIR 'Microsoft.NET/Framework/v4.0.30319/csc.exe'
$previousArch = $env:GOARCH
Push-Location $repo
try {
    New-Item -ItemType Directory -Force -Path $dest | Out-Null
    $env:GOARCH = $Arch
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o "$dest/ghosts.exe" ./cmd/ghosts
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
    $framework = Split-Path -Parent $compiler
    $wpf = Join-Path $framework 'WPF'
    $references = @("/r:$wpf\PresentationFramework.dll", "/r:$wpf\PresentationCore.dll", "/r:$wpf\WindowsBase.dll", "/r:$framework\System.Xaml.dll")
    & $compiler @references "/resource:$PSScriptRoot\Panel.xaml,Panel.xaml" "/resource:$PSScriptRoot\PortAlert.xaml,PortAlert.xaml" /nologo /target:winexe /platform:anycpu /optimize+ /utf8output /codepage:65001 "/out:$dest\GhostsTray.exe" /r:System.Windows.Forms.dll /r:System.Drawing.dll /r:System.Web.Extensions.dll "$PSScriptRoot\Tray.cs" "$PSScriptRoot\DesktopPanel.cs"
    if ($LASTEXITCODE -ne 0) { throw 'Tray build failed' }

    $docs = 'LICENSE', 'README.md', 'README.zh-TW.md'
    foreach ($doc in $docs) { Copy-Item -LiteralPath $doc -Destination $dest }
    Copy-Item -LiteralPath (Join-Path $repo 'packaging/install.ps1'), (Join-Path $PSScriptRoot 'install.cmd') -Destination $dest
    $files = foreach ($name in (@('ghosts.exe', 'GhostsTray.exe', 'install.cmd', 'install.ps1') + $docs)) { Join-Path $dest $name }
    $zip = Join-Path $OutputDirectory "ghosts_${Version}_windows_${Arch}.zip"
    Compress-Archive -LiteralPath $files -DestinationPath $zip -Force
    Write-Output $zip
} finally {
    $env:GOARCH = $previousArch
    Pop-Location
}
