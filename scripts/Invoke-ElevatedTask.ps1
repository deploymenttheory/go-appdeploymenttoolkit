<#
.SYNOPSIS
    Runs a task script elevated (UAC) and captures its output and exit code to
    files a non-elevated caller can read.

.DESCRIPTION
    Generic elevation helper for testing deployment workflows that require
    administrator rights (e.g. real msiexec installs) from a non-elevated
    terminal. Launches pwsh elevated via Start-Process -Verb RunAs, waits for
    completion, and writes:

        <OutputDir>\<TaskName>.out.txt        combined stdout/stderr
        <OutputDir>\<TaskName>.exitcode.txt   the task script's exit code

    The task script should end with `exit $LASTEXITCODE` (or an explicit
    `exit <n>`) so the deployment exit code propagates.

.EXAMPLE
    pwsh ./scripts/Invoke-ElevatedTask.ps1 -ScriptPath C:\work\tasks\install.ps1 -OutputDir C:\work\out
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$ScriptPath,
    [Parameter(Mandatory)][string]$OutputDir
)

$ErrorActionPreference = 'Stop'

$ScriptPath = (Resolve-Path $ScriptPath).Path
if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Force $OutputDir | Out-Null
}
$OutputDir = (Resolve-Path $OutputDir).Path
$taskName = [IO.Path]::GetFileNameWithoutExtension($ScriptPath)
$outFile = Join-Path $OutputDir "$taskName.out.txt"
$exitFile = Join-Path $OutputDir "$taskName.exitcode.txt"

# Stale results from a previous run must not be mistaken for this run's.
Remove-Item $outFile, $exitFile -ErrorAction SilentlyContinue

$inner = "& '$ScriptPath' *>&1 | Out-File -FilePath '$outFile' -Encoding utf8; `$code = `$LASTEXITCODE; if (`$null -eq `$code) { `$code = 0 }; Set-Content -Path '$exitFile' -Value `$code"

Start-Process pwsh -Verb RunAs -Wait -ArgumentList @(
    '-NoProfile', '-ExecutionPolicy', 'Bypass', '-Command', $inner
)

if (Test-Path $exitFile) {
    $code = (Get-Content $exitFile -Raw).Trim()
    Write-Host "Task '$taskName' finished with exit code $code. Output: $outFile"
    exit [int]$code
}
else {
    Write-Warning "Task '$taskName' produced no exit code file - it may have been cancelled at the UAC prompt."
    exit 1
}
