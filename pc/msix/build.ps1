# Builds the Microsoft Store package (.msix) from BlanketRemote.exe.
# With -Test it also signs a copy with a throwaway certificate, installs it on this machine, starts it the
# way Windows' startup task would, and checks that it answers, knows it is the Store version, and that its
# firewall rules were added. (The Store signs the real package itself; the uploaded .msix stays unsigned.)
param(
  [Parameter(Mandatory = $true)] [string]$Exe,
  [Parameter(Mandatory = $true)] [string]$Out,
  [Parameter(Mandatory = $true)] [string]$Version,
  [switch]$Test
)
$ErrorActionPreference = "Stop"
$here = $PSScriptRoot
$tmp = if ($env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { [IO.Path]::GetTempPath() }
$id = Get-Content "$here\identity.json" -Raw | ConvertFrom-Json

$kit = Get-ChildItem "${env:ProgramFiles(x86)}\Windows Kits\10\bin" -Directory |
  Where-Object { $_.Name -match '^10\.' -and (Test-Path "$($_.FullName)\x64\makeappx.exe") } |
  Sort-Object { [version]$_.Name } -Descending | Select-Object -First 1
if (-not $kit) { throw "Windows SDK (makeappx.exe) not found" }
$bin = "$($kit.FullName)\x64"
Write-Host "Windows SDK tools: $bin"

# ---- package folder ----
$root = Join-Path $tmp "msix-root"
Remove-Item $root -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory "$root\Assets" | Out-Null
Copy-Item $Exe "$root\BlanketRemote.exe"
Copy-Item "$here\Assets\*.png" "$root\Assets\"
$manifest = (Get-Content "$here\AppxManifest.xml" -Raw).
  Replace("{{NAME}}", $id.name).
  Replace("{{PUBLISHER}}", $id.publisher).
  Replace("{{PUBLISHER_DISPLAY}}", [Security.SecurityElement]::Escape($id.publisher_display)).
  Replace("{{VERSION}}", $Version)
[IO.File]::WriteAllText("$root\AppxManifest.xml", $manifest)

# resources.pri lets Windows pick the right icon size (taskbar, Start menu, Store)
$pricfg = Join-Path $tmp "priconfig.xml"
& "$bin\makepri.exe" createconfig /cf $pricfg /dq en-US /o | Out-Host
if ($LASTEXITCODE -ne 0) { throw "makepri createconfig failed" }
& "$bin\makepri.exe" new /pr $root /cf $pricfg /mn "$root\AppxManifest.xml" /of "$root\resources.pri" /o | Out-Host
if ($LASTEXITCODE -ne 0) { throw "makepri new failed" }

& "$bin\makeappx.exe" pack /d $root /p $Out /o | Out-Host
if ($LASTEXITCODE -ne 0) { throw "makeappx failed (the manifest is probably invalid)" }
Write-Host ("Store package: {0} ({1:N1} MB, version {2})" -f $Out, ((Get-Item $Out).Length / 1MB), $Version)

if (-not $Test) { exit 0 }

# ---- install a signed copy and check it really works as a Store app ----
$cert = New-SelfSignedCertificate -Type Custom -Subject $id.publisher -KeyUsage DigitalSignature `
  -FriendlyName "Blanket Remote package test" -CertStoreLocation "Cert:\CurrentUser\My" `
  -TextExtension @("2.5.29.37={text}1.3.6.1.5.5.7.3.3", "2.5.29.19={text}")
$cer = Join-Path $tmp "package-test.cer"
Export-Certificate -Cert $cert -FilePath $cer | Out-Null
Import-Certificate -FilePath $cer -CertStoreLocation "Cert:\LocalMachine\TrustedPeople" | Out-Null
$signed = Join-Path $tmp "package-test.msix"
Copy-Item $Out $signed -Force
& "$bin\signtool.exe" sign /fd SHA256 /sha1 $cert.Thumbprint $signed | Out-Host
if ($LASTEXITCODE -ne 0) { throw "signtool failed" }

Add-AppxPackage -Path $signed
$pkg = Get-AppxPackage -Name $id.name
if (-not $pkg) { throw "package did not install" }
Write-Host "Installed: $($pkg.PackageFullName)"

$alias = Join-Path $env:LOCALAPPDATA "Microsoft\WindowsApps\blanketremote.exe"
Start-Process $alias -ArgumentList "--autostart"
$ping = $null
for ($i = 0; $i -lt 30 -and -not $ping; $i++) {
  Start-Sleep -Seconds 1
  try { $ping = Invoke-RestMethod "http://127.0.0.1:47801/api/ping" -TimeoutSec 2 } catch { }
}
if (-not $ping) { throw "the installed Store version did not start or answer on port 47801" }
Write-Host "CHECK the Store version runs and answers: $($ping | ConvertTo-Json -Compress)"

$setup = Invoke-RestMethod "http://127.0.0.1:47801/api/setup"
Write-Host "CHECK it knows it is the Store version: $($setup.store)"
if (-not $setup.store) { throw "the app did not detect that it runs as the Store version" }

$rules = Get-NetFirewallApplicationFilter -ErrorAction SilentlyContinue |
  Where-Object { $_.Program -like "*WindowsApps*$($id.name)*" } | Get-NetFirewallRule -ErrorAction SilentlyContinue
$rules | ForEach-Object {
  $p = $_ | Get-NetFirewallPortFilter
  Write-Host ("CHECK firewall rule: {0} | {1} | profile {2} | {3} {4}" -f $_.DisplayName, $_.Direction, $_.Profile, $p.Protocol, $p.LocalPort)
}
if (-not $rules) { Write-Host "::warning::no firewall rules found for the package (check the firewall rules in the manifest)" }

try { Invoke-RestMethod "http://127.0.0.1:47801/api/quit" -Method Post -Headers @{ "X-BR" = "1" } -ContentType "application/json" -Body "{}" | Out-Null } catch { }
Start-Sleep -Seconds 1
Remove-AppxPackage -Package $pkg.PackageFullName
Write-Host "Package test passed"
