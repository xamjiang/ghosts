# Windows PowerShell 5.1+; install for the current user.
[CmdletBinding()]
param([string]$PackageDirectory)

& {
    $ErrorActionPreference = 'Stop'
    $dest = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'Programs\Ghosts'
    $names = @('ghosts.exe', 'GhostsTray.exe', 'ghost.exe')
    $stage = Join-Path $dest ('.install-' + [Guid]::NewGuid().ToString('N'))

    function Contains-Directory([string]$PathValue, [string]$Directory) {
        foreach ($entry in ($PathValue -split ';')) {
            $expanded = [Environment]::ExpandEnvironmentVariables($entry.Trim().Trim('"')).TrimEnd('\')
            if ($expanded -eq $Directory.TrimEnd('\')) { return $true }
        }
        return $false
    }

    function Add-UserPath {
        # Read the raw user value so entries such as %USERPROFILE% keep their meaning.
        $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
        try {
            $pathValue = [string]$key.GetValue('Path', '', 'DoNotExpandEnvironmentNames')
            if (!(Contains-Directory $pathValue $dest)) {
                $kind = if ($key.GetValueNames() -contains 'Path') { $key.GetValueKind('Path') } else { 'ExpandString' }
                $newPath = if ($pathValue) { $pathValue.TrimEnd(';') + ';' + $dest } else { $dest }
                $key.SetValue('Path', $newPath, $kind)
            }
        } finally { $key.Dispose() }
        if (!(Contains-Directory $env:Path $dest)) { $env:Path += ';' + $dest }
    }

    function Get-ReleaseFiles {
        $arch = switch ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()) {
            'X64' { 'amd64' }
            'Arm64' { 'arm64' }
            default { throw 'Ghosts supports x64 and ARM64 Windows.' }
        }
        $release = Invoke-RestMethod -Uri 'https://api.github.com/repos/xamjiang/ghosts/releases/latest'
        $tag = [string]$release.tag_name
        if ($tag -notmatch '^v[0-9][0-9A-Za-z._-]*$') { throw 'Could not identify the latest Ghosts release.' }
        $archive = 'ghosts_' + $tag.Substring(1) + '_windows_' + $arch + '.zip'
        if ($release.assets.name -notcontains $archive) { throw "Release $tag does not contain $archive." }
        $base = "https://github.com/xamjiang/ghosts/releases/download/$tag"
        $sums = (Invoke-WebRequest -UseBasicParsing "$base/checksums.txt").Content
        if ($sums -is [byte[]]) { $sums = [Text.Encoding]::ASCII.GetString($sums) }
        $pattern = '(?m)^([0-9a-fA-F]{64})[ \t]+\*?' + [regex]::Escape($archive) + '\r?$'
        $matchesFound = [regex]::Matches([string]$sums, $pattern)
        if ($matchesFound.Count -ne 1) { throw "Missing or ambiguous checksum for $archive." }
        $zip = Join-Path $stage 'release.zip'
        Invoke-WebRequest -UseBasicParsing "$base/$archive" -OutFile $zip
        if ((Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash -ne $matchesFound[0].Groups[1].Value) {
            throw 'The downloaded Windows package failed SHA-256 verification.'
        }
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $package = [IO.Compression.ZipFile]::OpenRead($zip)
        try {
            foreach ($name in 'ghosts.exe', 'GhostsTray.exe') {
                $entry = $package.GetEntry($name)
                if (!$entry) { throw "Missing $name in the release archive." }
                [IO.Compression.ZipFileExtensions]::ExtractToFile($entry, (Join-Path $stage $name))
            }
        } finally { $package.Dispose() }
    }

    $mutex = [Threading.Mutex]::new($false, 'Local\Ghosts.UserInstall')
    $locked = $false
    $installed = @()
    $keepRecovery = $false
    try {
        try { $locked = $mutex.WaitOne(0) } catch [Threading.AbandonedMutexException] { $locked = $true }
        if (!$locked) { throw 'Another Ghosts install is in progress.' }
        [IO.Directory]::CreateDirectory($stage) | Out-Null
        if ($PackageDirectory) {
            foreach ($name in 'ghosts.exe', 'GhostsTray.exe') {
                [IO.File]::Copy((Join-Path $PackageDirectory $name), (Join-Path $stage $name))
            }
        } else { Get-ReleaseFiles }
        [IO.File]::Copy((Join-Path $stage 'ghosts.exe'), (Join-Path $stage 'ghost.exe'))

        foreach ($name in $names) {
            $source = Join-Path $stage $name
            $target = Join-Path $dest $name
            if ([IO.File]::Exists($target)) {
                if ((Get-FileHash -LiteralPath $source).Hash -eq (Get-FileHash -LiteralPath $target).Hash) { continue }
                [IO.File]::Replace($source, $target, (Join-Path $stage "$name.old"))
            } else { [IO.File]::Move($source, $target) }
            $installed += $name
        }
        Add-UserPath
    } catch {
        $keepRecovery = $true
        foreach ($name in $installed) {
            $target = Join-Path $dest $name
            $backup = Join-Path $stage "$name.old"
            if ([IO.File]::Exists($backup)) { [IO.File]::Replace($backup, $target, (Join-Path $stage "$name.failed")) }
            else { [IO.File]::Delete($target) }
        }
        $keepRecovery = $false
        throw
    } finally {
        if ($locked) { $mutex.ReleaseMutex() }
        $mutex.Dispose()
        if (!$keepRecovery -and [IO.Directory]::Exists($stage)) {
            foreach ($name in $names) {
                [IO.File]::Delete((Join-Path $stage $name))
                [IO.File]::Delete((Join-Path $stage "$name.failed"))
                [IO.File]::Delete((Join-Path $stage "$name.old"))
            }
            [IO.File]::Delete((Join-Path $stage 'release.zip'))
            [IO.Directory]::Delete($stage)
        }
    }

    # Tell Explorer to pass the updated PATH to newly launched terminals.
    if (!('Ghosts.EnvironmentChange' -as [type])) {
        Add-Type -Namespace Ghosts -Name EnvironmentChange -MemberDefinition @'
[System.Runtime.InteropServices.DllImport("user32.dll", CharSet=System.Runtime.InteropServices.CharSet.Unicode)]
public static extern System.IntPtr SendMessageTimeout(System.IntPtr window, uint message, System.UIntPtr wParam, string value, uint flags, uint timeout, out System.UIntPtr result);
'@
    }
    $result = [UIntPtr]::Zero
    [void][Ghosts.EnvironmentChange]::SendMessageTimeout([IntPtr]65535, 26, [UIntPtr]::Zero, 'Environment', 2, 500, [ref]$result)
    Write-Host "Ghosts installed in $dest. Open a new CMD or PowerShell window and type ghost."
}
