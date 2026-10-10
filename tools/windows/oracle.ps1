# Windows GDI/GDI+ oracle run (ORACLES.md, "Windows GDI and GDI+"). It is
# not part of CI. On Windows, with Go installed:
#   $env:GOWEMF_WINDOWS_INPUTS = "$env:TEMP\in"; go test -run TestWriteWindowsInputs .
#   cd rendercheck; $env:GOWEMF_WINDOWS_INPUTS = "$env:TEMP\text"; go test -run TestWriteWindowsInputs .
#   powershell -File tools\windows\oracle.ps1 -In "$env:TEMP\in","$env:TEMP\text" -Out "$env:TEMP\out"
# then copy the output to .external/windows/out and run the comparisons with
# GOWEMF_WINDOWS_OUT set (TestCompareWindows*).
param(
    [Parameter(Mandatory = $true)][string[]]$In,
    [Parameter(Mandatory = $true)][string]$Out
)
$ErrorActionPreference = 'Stop'
Add-Type -ReferencedAssemblies System.Drawing -TypeDefinition (Get-Content -Raw (Join-Path $PSScriptRoot 'WinOracle.cs'))
New-Item -ItemType Directory -Force -Path $Out, (Join-Path $Out 'gdiplus'), (Join-Path $Out 'gdi'), (Join-Path $Out 'gdiwmf') | Out-Null

$gdiplus = Get-Item (Join-Path $env:WINDIR 'System32\gdiplus.dll')
@(
    "os: $([System.Environment]::OSVersion.VersionString)"
    "build: $((Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion').BuildLabEx)"
    "gdiplus.dll: $($gdiplus.VersionInfo.FileVersion)"
    "gdi32.dll: $((Get-Item (Join-Path $env:WINDIR 'System32\gdi32.dll')).VersionInfo.FileVersion)"
    "dpi: $([System.Drawing.Graphics]::FromHwnd([IntPtr]::Zero).DpiX)"
) | Set-Content -Encoding ascii (Join-Path $Out 'environment.txt')

[WinOracle]::Hatches() | Set-Content -Encoding ascii (Join-Path $Out 'hatches.txt')
foreach ($cp in 932, 936, 949, 950) {
    [WinOracle]::DumpCodePage([uint32]$cp, (Join-Path $Out "mbtowc-$cp.txt"))
}

$errors = Join-Path $Out 'errors.txt'
$recorded = Join-Path $Out 'recorded'
New-Item -ItemType Directory -Force -Path $recorded | Out-Null
[WinOracle]::RecordScenes($recorded, $In[0]) | Set-Content -Encoding ascii (Join-Path $Out 'record-errors.txt')
$scenes = @()
foreach ($f in Get-ChildItem $recorded -Filter 'rec-*.emf') {
    $w = 96; $h = 64
    if ($f.Name -eq 'rec-inner.emf') { $w = 40; $h = 20 }
    $scenes += [pscustomobject]@{ file = $f.Name; w = $w; h = $h }
}
$scenes | ConvertTo-Json | Set-Content -Encoding ascii (Join-Path $recorded 'manifest.json')
foreach ($dir in @($In) + $recorded) {
    $manifest = Get-Content -Raw (Join-Path $dir 'manifest.json') | ConvertFrom-Json
    foreach ($s in $manifest) {
        $file = Join-Path $dir $s.file
        try {
            [WinOracle]::RenderGdiPlus($file, $s.w, $s.h, (Join-Path $Out "gdiplus\$($s.file).png"))
        } catch {
            Add-Content -Path $errors -Value "gdiplus $($s.file): $_"
        }
        try {
            [void][WinOracle]::RenderGdi($file, $s.w, $s.h, (Join-Path $Out "gdi\$($s.file).png"))
        } catch {
            Add-Content -Path $errors -Value "gdi $($s.file): $_"
        }
        try {
            [void][WinOracle]::RenderGdiWmf($file, $s.w, $s.h, (Join-Path $Out "gdiwmf\$($s.file).png"))
        } catch {
            Add-Content -Path $errors -Value "gdiwmf $($s.file): $_"
        }
    }
}
