$ErrorActionPreference = 'Stop'
$script:RepoRoot = Split-Path $PSScriptRoot -Parent
Set-Location $script:RepoRoot
function Compose {
    & docker compose --env-file infra/compose/.env -f infra/compose/compose.yaml @args
    if ($LASTEXITCODE) { throw "Compose operation failed (exit $LASTEXITCODE)" }
}
function Sql([string]$Node, [string]$Statement) {
    $result = Compose exec -T -e 'PGOPTIONS=-c statement_timeout=15000' "db-$Node" psql -U erm -d erm -At -v ON_ERROR_STOP=1 -c $Statement
    return ($result -join "`n").Trim()
}
function Internal([string]$Path, [string]$Method='GET', $Body=$null) {
    $entry = Get-Content infra/compose/.env | Where-Object { $_ -like 'ERM_INTERNAL_SECRET=*' }
    $secret = $entry.Substring('ERM_INTERNAL_SECRET='.Length)
    $params = @{Uri="http://127.0.0.1:8081/internal/$Path";Method=$Method;Headers=@{'X-Internal-Secret'=$secret};TimeoutSec=8}
    if ($null -ne $Body) {$params.ContentType='application/json';$params.Body=($Body | ConvertTo-Json -Compress)}
    Invoke-RestMethod @params
}
function Wait-Condition([scriptblock]$Check, [string]$Description, [int]$Seconds=60) {
    $deadline=(Get-Date).AddSeconds($Seconds)
    do {if (& $Check) {return};Start-Sleep -Seconds 1} while ((Get-Date) -lt $deadline)
    throw "Timed out: $Description"
}
function Enable-Sync([string]$Primary,[string]$Standby) {
    Wait-Condition { (Sql $Primary "SELECT count(*) FROM pg_stat_replication WHERE application_name='node-$Standby' AND state='streaming'") -eq '1' } 'standby streaming'
    Sql $Primary "ALTER SYSTEM SET synchronous_standby_names = 'FIRST 1 (`"node-$Standby`")'" | Out-Null
    Sql $Primary 'ALTER SYSTEM SET synchronous_commit = remote_apply' | Out-Null
    Sql $Primary 'SELECT pg_reload_conf()' | Out-Null
    Wait-Condition { (Sql $Primary "SELECT count(*) FROM pg_stat_replication WHERE application_name='node-$Standby' AND sync_state='sync'") -eq '1' } 'synchronous standby'
}
