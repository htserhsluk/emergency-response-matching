param([Parameter(Mandatory)][ValidateRange(0,2)][int]$Shard,[Parameter(Mandatory)][ValidateSet('a','b')][string]$From)
. "$PSScriptRoot/lab-common.ps1"
$other=if ($From -eq 'a') {'b'} else {'a'}
$old="$Shard$From"; $target="$Shard$other"
$route=(Internal 'routes')[$Shard]
if ($route.nodeId -eq "node-$target") {Write-Output "Route already activated: $target";return}
if ($route.nodeId -ne "node-$old") {throw 'Unexpected active node'}
New-Item -ItemType Directory -Force .local/recovery | Out-Null
$journalPath=".local/recovery/shard-$Shard-from-$From-v$($route.version).json"
if (Test-Path $journalPath) {$journal=Get-Content $journalPath -Raw | ConvertFrom-Json
    if ($journal.version -ne $route.version) {throw 'Recovery journal version differs; inspect state before proceeding'}
} else {
    if ((Sql $old "SELECT count(*) FROM pg_stat_replication WHERE application_name='node-$target' AND state='streaming' AND sync_state='sync'") -ne '1') {throw 'No eligible synchronous standby'}
    if ((Sql $target 'SELECT pg_is_in_recovery()') -ne 't') {throw 'Target is not a standby'}
    $journal=@{version=$route.version;lsn=(Sql $old 'SELECT pg_current_wal_flush_lsn()');systemId=(Sql $old 'SELECT system_identifier FROM pg_control_system()')}
    $journal | ConvertTo-Json | Set-Content $journalPath
}
foreach ($service in @("node-$old","db-$old")) {
    $id=Compose ps -a -q $service
    if (!$id) {throw "Missing container $service; cannot prove fencing"}
    & docker update --restart=no $id | Out-Null
    if ($LASTEXITCODE) {throw 'Failed to disable restart'}
}
Compose stop "node-$old" "db-$old"
foreach ($service in @("node-$old","db-$old")) {
    $id=Compose ps -a -q $service
    $state=& docker inspect --format '{{.State.Running}} {{.HostConfig.RestartPolicy.Name}}' $id
    if ($LASTEXITCODE -or $state -ne 'false no') {throw 'Fencing not confirmed'}
}
if ((Sql $target 'SELECT system_identifier FROM pg_control_system()') -ne $journal.systemId) {throw 'Standby belongs to a different cluster'}
if ((Sql $target 'SELECT pg_is_in_recovery()') -eq 't') {
    if ((Sql $target "SELECT pg_last_wal_replay_lsn() >= '$($journal.lsn)'::pg_lsn") -ne 't') {throw 'Standby has not replayed required WAL'}
    Sql $target 'SELECT pg_promote(true,30)' | Out-Null
}
if ((Sql $target 'SELECT pg_is_in_recovery()') -ne 'f') {throw 'Promotion not confirmed'}
Sql $target "ALTER SYSTEM SET synchronous_standby_names = ''" | Out-Null
Sql $target 'ALTER SYSTEM SET synchronous_commit = on' | Out-Null
Sql $target 'SELECT pg_reload_conf()' | Out-Null
Wait-Condition {
    $nodes=Internal 'nodes';$oldReport=$nodes."node-$old";$newReport=$nodes."node-$target"
    $oldExpired=(!$oldReport) -or (([DateTimeOffset]::UtcNow-[DateTimeOffset]$oldReport.received).TotalSeconds -gt 9)
    $oldExpired -and $newReport.report.primary -and $newReport.report.reachable
} 'old route expiry and promoted heartbeat'
Internal "routes/$Shard" 'PUT' @{nodeId="node-$target";expectedVersion=$route.version}
Write-Output "Shard $Shard uses $target in explicit single-copy mode. Keep $old fenced until rejoin."
