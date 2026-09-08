# Installs Diskette for the current user: copies diskette.exe into
# %LOCALAPPDATA%\Diskette and creates a Start Menu shortcut. diskette.exe
# already carries its own icon (embedded via go-winres at build time), so
# the shortcut picks that up automatically.
#
# Usage (from a PowerShell prompt, no admin rights needed):
#   .\install.ps1 [-ExePath .\diskette-windows-amd64.exe]
param(
	[string]$ExePath = "$PSScriptRoot\..\..\dist\diskette-windows-amd64.exe"
)

if (-not (Test-Path $ExePath)) {
	Write-Error "Binary not found at $ExePath -- build it first, or pass -ExePath."
	exit 1
}

$installDir = "$env:LOCALAPPDATA\Diskette"
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
Copy-Item -Force $ExePath "$installDir\diskette.exe"

$startMenu = "$env:APPDATA\Microsoft\Windows\Start Menu\Programs"
$shortcut = (New-Object -ComObject WScript.Shell).CreateShortcut("$startMenu\Diskette.lnk")
$shortcut.TargetPath = "$installDir\diskette.exe"
$shortcut.IconLocation = "$installDir\diskette.exe"
$shortcut.Save()

Write-Host "Installed to $installDir"
Write-Host "Find `"Diskette`" in the Start menu, or search for it directly."
