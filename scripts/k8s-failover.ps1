param([Parameter(Mandatory)][ValidateRange(0,2)][int]$Shard,[Parameter(Mandatory)][ValidateSet('a','b')][string]$From)
. "$PSScriptRoot/k8s-common.ps1"
$other=if ($From -eq 'a'){'b'}else{'a'};$old="$Shard$From";$target="$Shard$other"
$route=(Kube-Internal routes)[$Shard]
if ($route.nodeId -eq "node-$target"){Write-Output 'Route already promoted';return}
if ($route.nodeId -ne "node-$old"){throw 'Unexpected active node'}
New-Item -ItemType Directory -Force .local/recovery | Out-Null
$journalPath=".local/recovery/k8s-$Shard-$From-v$($route.version).json"
if (Test-Path $journalPath){$journal=Get-Content $journalPath -Raw|ConvertFrom-Json}else{
    if ((Kube-Sql $old "SELECT count(*) FROM pg_stat_replication WHERE application_name='node-$target' AND state='streaming' AND sync_state='sync'") -ne '1'){throw 'No synchronous candidate'}
    $journal=@{lsn=(Kube-Sql $old 'SELECT pg_current_wal_flush_lsn()');systemId=(Kube-Sql $old 'SELECT system_identifier FROM pg_control_system()')}
    $journal|ConvertTo-Json|Set-Content $journalPath
}
Kube scale "statefulset/node-$old" --replicas=0
Kube wait --for=delete "pod/node-$old-0" --timeout=90s
$replicas=Kube get "statefulset/node-$old" -o 'jsonpath={.spec.replicas}'
if ($replicas -ne '0'){throw 'Controller is not fenced'}
if ((Kube-Sql $target 'SELECT system_identifier FROM pg_control_system()') -ne $journal.systemId){throw 'Wrong standby cluster'}
if ((Kube-Sql $target 'SELECT pg_is_in_recovery()') -eq 't'){
    if ((Kube-Sql $target "SELECT pg_last_wal_replay_lsn() >= '$($journal.lsn)'::pg_lsn") -ne 't'){throw 'Insufficient replay'}
    Kube-Sql $target 'SELECT pg_promote(true,30)'|Out-Null
}
Kube-Sql $target "ALTER SYSTEM SET synchronous_standby_names = ''"|Out-Null
Kube-Sql $target 'ALTER SYSTEM SET synchronous_commit = on'|Out-Null
Kube-Sql $target 'SELECT pg_reload_conf()'|Out-Null
Kube-Wait {$nodes=Kube-Internal nodes;$a=$nodes."node-$old";$b=$nodes."node-$target";((!$a)-or(([DateTimeOffset]::UtcNow-[DateTimeOffset]$a.received).TotalSeconds-gt 9))-and $b.report.primary -and $b.report.reachable} 'fenced expiry and promoted heartbeat'
Kube-Internal "routes/$Shard" PUT @{nodeId="node-$target";expectedVersion=$route.version}
Write-Output "Kubernetes shard $Shard is explicitly single-copy on $target."
