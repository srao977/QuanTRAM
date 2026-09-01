#Requires -Version 5.1
<#
.SYNOPSIS
  Start QuanTRAM increment-1 ingestion, or smoke-test it.

.DESCRIPTION
  Sets QUANTRAM_* from parameters, uses ALPACA_API_KEY / ALPACA_API_SECRET
  already in your session, and runs the Go server from the repo root.
  Does not print credentials.

.EXAMPLE
  .\scripts\Start-QuantramIngestion.ps1
  Alpaca test feed, FAKEPACA, server in this window.

.EXAMPLE
  .\scripts\Start-QuantramIngestion.ps1 -SmokeTest
  Start server, wait until healthy, stream one bar, stop the server.

.EXAMPLE
  .\scripts\Start-QuantramIngestion.ps1 -Feed iex -Symbols AAPL

.EXAMPLE
  .\scripts\Start-QuantramIngestion.ps1 -Source csv -SmokeTest
#>
[CmdletBinding()]
param(
    [ValidateSet("alpaca", "csv")]
    [string]$Source = "alpaca",

    [ValidateSet("iex", "test")]
    [string]$Feed = "test",

    [string[]]$Symbols = @(),

    [string]$CsvPath = "AAPL_1min_firstratedata.csv",

    [string]$Port = "50051",

    [switch]$SmokeTest,

    [ValidateSet("stream", "decisions", "health", "ready", "source", "window", "gapfill")]
    [string]$Operation = "stream",

    [int]$MaxBars = 1,

    [string]$Timeout = "4m"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go is not on PATH. Install Go or open a shell where 'go version' works."
}

if (-not $Symbols) {
    if ($Source -eq "csv") {
        $Symbols = @("AAPL")
    }
    elseif ($Feed -eq "test") {
        $Symbols = @("FAKEPACA")
    }
    else {
        $Symbols = @("AAPL")
    }
}

$symbolsCsv = (($Symbols -join ",") -split "," |
    ForEach-Object { $_.Trim() } |
    Where-Object { $_ }) -join ","

if (-not $symbolsCsv) {
    throw "Symbols must contain at least one non-empty symbol."
}

if ($Source -eq "alpaca") {
    $hasKey = [string]::IsNullOrWhiteSpace($env:ALPACA_API_KEY) -eq $false
    $hasSecret = ([string]::IsNullOrWhiteSpace($env:ALPACA_API_SECRET) -eq $false) -or
        ([string]::IsNullOrWhiteSpace($env:ALPACA_SECRET_KEY) -eq $false)
    if (-not $hasKey -or -not $hasSecret) {
        throw "Set ALPACA_API_KEY and ALPACA_API_SECRET (or ALPACA_SECRET_KEY) in this session first."
    }
}

$env:QUANTRAM_SOURCE = $Source
$env:QUANTRAM_FEED = $Feed
$env:QUANTRAM_SYMBOLS = $symbolsCsv
$env:QUANTRAM_CSV_PATH = $CsvPath
$env:GRPC_PORT = $Port

Write-Host "QuanTRAM ingestion  source=$Source  feed=$Feed  symbols=$symbolsCsv  port=$Port"

function Invoke-IngestClient {
    param(
        [string]$ClientOperation,
        [int]$ClientMaxBars,
        [string]$ClientTimeout
    )
    & go run ./cmd/quantram-ingest-client `
        -address "localhost:$Port" `
        -operation $ClientOperation `
        -symbols $symbolsCsv `
        -max-bars $ClientMaxBars `
        -timeout $ClientTimeout
    if ($LASTEXITCODE -ne 0) {
        throw "ingest client $ClientOperation failed with exit $LASTEXITCODE"
    }
}

if (-not $SmokeTest) {
    Write-Host "Starting server (Ctrl+C to stop). In another terminal run:"
    Write-Host "  go run ./cmd/quantram-ingest-client -operation source"
    Write-Host "  go run ./cmd/quantram-ingest-client -operation stream -symbols $symbolsCsv -max-bars $MaxBars -timeout $Timeout"
    & go run ./cmd/quantram-server
    exit $LASTEXITCODE
}

$server = $null
try {
    $server = Start-Process -FilePath "go" -ArgumentList @("run", "./cmd/quantram-server") `
        -WorkingDirectory $repoRoot `
        -PassThru `
        -NoNewWindow

    $deadline = (Get-Date).AddSeconds(30)
    $healthy = $false
    while ((Get-Date) -lt $deadline) {
        Start-Sleep -Seconds 2
        if ($server.HasExited) {
            throw "server exited early with code $($server.ExitCode)"
        }
        try {
            $output = & go run ./cmd/quantram-ingest-client -address "localhost:$Port" -operation source 2>&1 |
                Out-String
            if ($output -match "FEED_STATE_HEALTHY") {
                Write-Host $output.Trim()
                $healthy = $true
                break
            }
        }
        catch {
            # server still binding
        }
    }
    if (-not $healthy) {
        throw "server did not become HEALTHY within 30 seconds"
    }

    Invoke-IngestClient -ClientOperation "ready" -ClientMaxBars $MaxBars -ClientTimeout $Timeout
    Invoke-IngestClient -ClientOperation $Operation -ClientMaxBars $MaxBars -ClientTimeout $Timeout
    Write-Host "Smoke test passed."
}
finally {
    if ($null -ne $server -and -not $server.HasExited) {
        Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue
        Get-CimInstance Win32_Process -Filter "Name = 'go.exe'" -ErrorAction SilentlyContinue |
            Where-Object { $_.CommandLine -match "quantram-server" } |
            ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
    }
}
