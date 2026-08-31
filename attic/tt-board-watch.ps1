# tt-board-watch.ps1 — event-driven Kanban board -> task-frontmatter sync (Windows), hardened.
#
# Watches <vault>/kanban/*-board.md. When you drag a card to another lane in Obsidian, the board
# file changes; this picks the new lane up and writes it into the task's `status:` frontmatter via
# tt-board-apply.sh (board -> frontmatter, one direction).
#
# Why it can't loop: tt-board-apply.sh writes ONLY task files (tasks/<proj>/*.md), never the board,
# so the watcher never triggers itself. The board is NOT regenerated here on purpose — the card is
# already where you dropped it; normalization happens on the next agent edit (PostToolUse refresh
# hook) or `/board refresh`.
#
# SAFETY (learned the hard way — Obsidian rewrites/corrupts the board file while it is open):
#   1. Sanity gate  — a board with missing lane headers or absurdly long lines (corruption) is
#                      skipped, never applied.
#   2. Bulk guard   — a human drags ONE card at a time. If a single change would flip more than
#                      -MaxChanges tasks at once, it is treated as a reformat/corruption and SKIPPED
#                      (logged, not applied). This is the guard that would have caught the 16-task
#                      mass-flip incident.
#   3. Dry-run first — we count the planned changes via TT_DRY_RUN before writing anything.
#
# Usage:
#   pwsh -File tt-board-watch.ps1                                  # vault from ~/.claude/task-tracker.json
#   pwsh -File tt-board-watch.ps1 -Vault 'D:/task tracker' -MaxChanges 3 -DebounceMs 2500
param(
  [string]$Vault,
  [int]$MaxChanges = 3,
  [int]$DebounceMs = 2500
)
$ErrorActionPreference = 'Stop'

# --- resolve vault -----------------------------------------------------------
if (-not $Vault) {
  $reg = Join-Path $env:USERPROFILE '.claude\task-tracker.json'
  if (Test-Path $reg) {
    try { $Vault = (Get-Content $reg -Raw | ConvertFrom-Json).vault } catch {}
  }
}
if (-not $Vault) { Write-Error 'tt-board-watch: vault not given and not found in ~/.claude/task-tracker.json'; exit 2 }
$Vault   = $Vault.TrimEnd('/','\')
$kanban  = Join-Path $Vault 'kanban'
if (-not (Test-Path $kanban)) { Write-Error "tt-board-watch: no kanban dir at $kanban"; exit 2 }

# --- locate bash + the apply script -----------------------------------------
$bash = $null
foreach ($c in @('C:\Program Files\Git\usr\bin\bash.exe','C:\Program Files\Git\bin\bash.exe')) {
  if (Test-Path $c) { $bash = $c; break }
}
if (-not $bash) { $g = Get-Command bash -ErrorAction SilentlyContinue; if ($g) { $bash = $g.Source } }
if (-not $bash) { Write-Error 'tt-board-watch: bash.exe not found (install Git for Windows)'; exit 2 }

$applySh = Join-Path $env:USERPROFILE '.claude\lib\tt-board-apply.sh'
if (-not (Test-Path $applySh)) { Write-Error "tt-board-watch: apply script not found at $applySh"; exit 2 }
$applyArg = ($applySh -replace '\\','/')   # bash on Windows accepts drive-letter paths with forward slashes
$vaultArg = ($Vault   -replace '\\','/')

# --- logging -----------------------------------------------------------------
$lockDir = Join-Path $Vault '.locks'
if (-not (Test-Path $lockDir)) { New-Item -ItemType Directory -Path $lockDir -Force | Out-Null }
$logFile = Join-Path $lockDir 'board-watch.log'
function Log($msg) {
  $line = ('{0}  {1}' -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'), $msg)
  Write-Host $line
  Add-Content -Path $logFile -Value $line -Encoding UTF8
}

# --- board sanity gate -------------------------------------------------------
$RequiredLanes = @('## Backlog','## Ready','## In Progress','## Needs Input','## Blocked','## Hold','## Failed','## Done','## Canceled')
function Test-BoardsSane {
  foreach ($b in (Get-ChildItem (Join-Path $kanban '*-board.md') -ErrorAction SilentlyContinue)) {
    try { $lines = Get-Content $b.FullName -ErrorAction Stop } catch { Log "[gate] cannot read $($b.Name): $_"; return $false }
    $text = $lines -join "`n"
    foreach ($lane in $RequiredLanes) {
      if ($text -notmatch [regex]::Escape($lane)) { Log "[gate] $($b.Name): missing lane '$lane' -> board looks malformed, skipping"; return $false }
    }
    foreach ($l in $lines) {
      if ($l.Length -gt 500) { Log "[gate] $($b.Name): line of $($l.Length) chars -> corruption, skipping"; return $false }
    }
  }
  return $true
}

# --- single instance (machine-global) ---------------------------------------
$mutex = New-Object System.Threading.Mutex($false, 'Global\tt-board-watch')
if (-not $mutex.WaitOne(0)) { Log 'already running — exiting'; exit 0 }

# --- run the board -> frontmatter sync, gated -------------------------------
function Invoke-Sync {
  if (-not (Test-BoardsSane)) { return }                 # gate 1: malformed board
  # gate 2/3: dry-run to count planned changes without writing
  $env:TT_DRY_RUN = '1'
  try { $dry = & $bash -l $applyArg $vaultArg 2>&1 } finally { Remove-Item Env:TT_DRY_RUN -ErrorAction SilentlyContinue }
  $planned = @($dry | Where-Object { $_ -match ' -> ' })
  $count = $planned.Count
  if ($count -eq 0) { return }                            # nothing changed
  if ($count -gt $MaxChanges) {
    Log "[guard] $count tasks would change at once (> $MaxChanges) — looks like a reformat/corruption, not a drag. SKIPPING."
    foreach ($p in $planned) { Log "[guard]  skipped:$p" }
    return
  }
  # safe: apply for real
  try {
    $out = & $bash -l $applyArg $vaultArg 2>&1
    foreach ($l in $out) { if ("$l".Trim()) { Log "[apply] $l" } }
  } catch { Log "[apply] ERROR: $($_.Exception.Message)" }
}

# --- watch -------------------------------------------------------------------
$fsw = New-Object System.IO.FileSystemWatcher $kanban, '*.md'
$fsw.NotifyFilter = [System.IO.NotifyFilters]::LastWrite -bor `
                    [System.IO.NotifyFilters]::Size      -bor `
                    [System.IO.NotifyFilters]::FileName
$fsw.IncludeSubdirectories = $false
$fsw.EnableRaisingEvents    = $true

Log "watching $kanban\*-board.md (debounce ${DebounceMs}ms, max-changes ${MaxChanges}, pid $PID)"
try {
  while ($true) {
    $r = $fsw.WaitForChanged([System.IO.WatcherChangeTypes]::All)   # blocks until any change
    if ($r.Name -notlike '*-board.md') { continue }
    # debounce: keep waiting until there's a quiet gap of DebounceMs (Obsidian writes in bursts)
    do { $q = $fsw.WaitForChanged([System.IO.WatcherChangeTypes]::All, $DebounceMs) }
    while (-not $q.TimedOut)
    Log "board changed ($($r.Name)) -> evaluating"
    Invoke-Sync
  }
} finally {
  $fsw.Dispose()
  $mutex.ReleaseMutex()
}
