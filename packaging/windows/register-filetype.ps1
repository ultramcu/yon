# Associates .yon files with Yon for the current user (no admin rights needed),
# so double-clicking a collection in Explorer opens it in Yon. Windows passes the
# file path on the command line, which Yon already opens.
#
# Run from the folder containing yon.exe:
#   powershell -ExecutionPolicy Bypass -File register-filetype.ps1
# Or point it at a specific exe:
#   powershell -ExecutionPolicy Bypass -File register-filetype.ps1 -Exe "C:\Apps\Yon\yon.exe"
param(
    [string]$Exe = (Join-Path $PSScriptRoot 'yon.exe')
)

if (-not (Test-Path $Exe)) {
    Write-Error "yon.exe not found at '$Exe'. Pass -Exe <path-to-yon.exe>."
    exit 1
}
$Exe = (Resolve-Path $Exe).Path

$progId = 'Yon.Collection'
$classes = 'HKCU:\Software\Classes'

# Map the .yon extension to our ProgID.
New-Item -Path "$classes\.yon" -Force | Out-Null
Set-ItemProperty -Path "$classes\.yon" -Name '(default)' -Value $progId

# Describe the ProgID, its icon, and the open command (%1 = the clicked file).
New-Item -Path "$classes\$progId" -Force | Out-Null
Set-ItemProperty -Path "$classes\$progId" -Name '(default)' -Value 'Yon Collection'

New-Item -Path "$classes\$progId\DefaultIcon" -Force | Out-Null
Set-ItemProperty -Path "$classes\$progId\DefaultIcon" -Name '(default)' -Value "`"$Exe`",0"

New-Item -Path "$classes\$progId\shell\open\command" -Force | Out-Null
Set-ItemProperty -Path "$classes\$progId\shell\open\command" -Name '(default)' -Value "`"$Exe`" `"%1`""

Write-Host "Registered .yon -> $Exe (current user)."
Write-Host "Sign out/in or restart Explorer if the association does not take effect immediately."
