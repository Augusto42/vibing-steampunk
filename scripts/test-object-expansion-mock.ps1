[CmdletBinding()]
param(
    [int]$Port = 50080,
    [string]$GoExe = ""
)

$ErrorActionPreference = "Stop"
$repoRoot = [IO.Path]::GetFullPath((Split-Path -Parent $PSScriptRoot))

if (-not $GoExe) {
    $goCommand = Get-Command go -ErrorAction SilentlyContinue
    if ($goCommand) {
        $GoExe = $goCommand.Source
    } else {
        $userProfile = [Environment]::GetFolderPath("UserProfile")
        $portableGo = Join-Path $userProfile ".cache\codex-go\go1.25.13\go\bin\go.exe"
        if (Test-Path -LiteralPath $portableGo) {
            $GoExe = $portableGo
        }
    }
}
if (-not $GoExe -or -not (Test-Path -LiteralPath $GoExe)) {
    throw "Go was not found. Pass -GoExe with the path to go.exe."
}

$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$runDirectory = [IO.Path]::GetFullPath((Join-Path $tempRoot ("vsp-mock-sap-" + [guid]::NewGuid().ToString("N"))))
if (-not $runDirectory.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Refusing to use temporary directory outside the system temp root: $runDirectory"
}

New-Item -ItemType Directory -Path $runDirectory | Out-Null
$vspExe = Join-Path $runDirectory "vsp.exe"
$mockExe = Join-Path $runDirectory "vsp-mock-sap.exe"
$mockStdout = Join-Path $runDirectory "mock-stdout.log"
$mockStderr = Join-Path $runDirectory "mock-stderr.log"
$mockProcess = $null

try {
    Push-Location $repoRoot
    try {
        & $GoExe build -o $vspExe ./cmd/vsp
        if ($LASTEXITCODE -ne 0) { throw "Failed to build vsp" }
        & $GoExe build -o $mockExe ./cmd/vsp-mock-sap
        if ($LASTEXITCODE -ne 0) { throw "Failed to build vsp-mock-sap" }
    } finally {
        Pop-Location
    }

    $baseURL = "http://127.0.0.1:$Port"
    $mockProcess = Start-Process -FilePath $mockExe `
        -ArgumentList @("-listen", "127.0.0.1:$Port") `
        -WorkingDirectory $runDirectory `
        -WindowStyle Hidden `
        -RedirectStandardOutput $mockStdout `
        -RedirectStandardError $mockStderr `
        -PassThru

    $healthy = $false
    foreach ($attempt in 1..40) {
        try {
            $health = Invoke-RestMethod -Uri "$baseURL/healthz" -TimeoutSec 1
            if ($health.status -eq "ok") {
                $healthy = $true
                break
            }
        } catch {
            Start-Sleep -Milliseconds 250
        }
    }
    if (-not $healthy) {
        $details = ""
        if (Test-Path -LiteralPath $mockStderr) {
            $details = Get-Content -LiteralPath $mockStderr -Raw
        }
        throw "Synthetic SAP server did not become healthy. $details"
    }

    $config = @"
{
  "default": "mock",
  "systems": {
    "mock": {
      "url": "$baseURL",
      "user": "SYNTHETIC_USER",
      "password": "synthetic-password",
      "client": "001",
      "language": "EN",
      "allowed_packages": ["`$TMP"]
    }
  }
}
"@
    [IO.File]::WriteAllText((Join-Path $runDirectory ".vsp.json"), $config)

    Push-Location $runDirectory
    try {
        $enhancement = & $vspExe source ENHO ZSYNTHETIC_ENHO
        if ($LASTEXITCODE -ne 0 -or -not ($enhancement -match "ENHANCEMENT 1")) {
            throw "ENHO end-to-end read failed"
        }

        $dynpro = & $vspExe source DYNP ZSYNTHETIC_APP/100
        if ($LASTEXITCODE -ne 0 -or -not ($dynpro -match '"screen": "0100"')) {
            throw "DYNP end-to-end read failed"
        }

        $updatedSource = "INCLUDE zsynthetic_include.`nDATA gv_synthetic_value TYPE i VALUE 7."
        $updatedSource | & $vspExe source write INCL ZSYNTHETIC_INCLUDE
        if ($LASTEXITCODE -ne 0) {
            throw "INCL end-to-end write failed"
        }

        $readBack = & $vspExe source INCL ZSYNTHETIC_INCLUDE
        if ($LASTEXITCODE -ne 0 -or -not ($readBack -match "gv_synthetic_value")) {
            throw "INCL end-to-end read-back failed"
        }
    } finally {
        Pop-Location
    }

    Write-Host "PASS: ENHO read, DYNP WebSocket read, and INCL write/read-back succeeded against the synthetic SAP server."
} finally {
    if ($mockProcess -and -not $mockProcess.HasExited) {
        Stop-Process -Id $mockProcess.Id
        Wait-Process -Id $mockProcess.Id -ErrorAction SilentlyContinue
    }
    if (Test-Path -LiteralPath $runDirectory) {
        $resolvedRunDirectory = [IO.Path]::GetFullPath($runDirectory)
        if ($resolvedRunDirectory.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase)) {
            Remove-Item -LiteralPath $resolvedRunDirectory -Recurse -Force
        }
    }
}
