. "$PSScriptRoot/k8s-common.ps1"
. "$PSScriptRoot/fixture-requests.ps1"
$api=$null;$control=$null;$paused=$false;$restorePair=$false
try {
    $api=Start-KubeForward api 18080 8080
    $control=Start-KubeForward coordinator 18081 8081
    $route=(Kube-Internal routes)[0];$primary=$route.nodeId.Replace('node-','');$from=$primary.Substring(1)
    $side=if($from -eq 'a'){'b'}else{'a'};$standby="0$side"
    $tag=[Guid]::NewGuid().ToString('N')
    $before=New-ShardRequest http://127.0.0.1:18080 0 "$tag-before"
    $healthy=New-ShardRequest http://127.0.0.1:18080 1 "$tag-healthy"
    Kube scale "statefulset/node-$standby" --replicas=0
    $restorePair=$true
    Kube wait --for=delete "pod/node-$standby-0" --timeout=90s
    $failed=$false
    try {New-ShardRequest http://127.0.0.1:18080 0 "$tag-unconfirmed"|Out-Null} catch {$failed=$true}
    if(!$failed){throw 'Synchronous write incorrectly confirmed without standby'}
    # Explicit operator decision: primary is alive, acknowledged history retained;
    # temporarily accept single-copy writes until the same standby returns.
    Kube-Sql $primary "ALTER SYSTEM SET synchronous_standby_names = ''"|Out-Null
    Kube-Sql $primary 'ALTER SYSTEM SET synchronous_commit = on'|Out-Null
    Kube-Sql $primary 'SELECT pg_reload_conf()'|Out-Null
    $during=New-ShardRequest http://127.0.0.1:18080 0 "$tag-degraded"
    Kube scale "statefulset/node-$standby" --replicas=1
    Kube rollout status "statefulset/node-$standby" --timeout=120s
    Kube-Sync $primary $standby
    if((Kube-Sql $standby "SELECT count(*) FROM requests WHERE id='$($during.id)'") -ne '1'){throw 'Degraded write missing from recovered standby'}
    Write-Output 'PASS: standby loss refuses unconfirmed success; explicit degraded writes synchronize on recovery.'
    Kube scale "statefulset/node-$primary" "statefulset/node-$standby" --replicas=0
    Kube wait --for=delete "pod/node-$primary-0" "pod/node-$standby-0" --timeout=90s
    $watch=[Diagnostics.Stopwatch]::StartNew()
    $response=Invoke-WebRequest "http://127.0.0.1:18080/requests/$($before.id)" -SkipHttpErrorCheck -TimeoutSec 8
    if([int]$response.StatusCode -ne 503 -or $watch.Elapsed.TotalSeconds -gt 8){throw 'Both-copy outage did not return bounded 503'}
    $got=Invoke-RestMethod "http://127.0.0.1:18080/requests/$($healthy.id)" -TimeoutSec 8
    if($got.id -ne $healthy.id){throw 'Healthy shard became inaccessible'}
    Kube scale "statefulset/node-$primary" "statefulset/node-$standby" --replicas=1
    Kube rollout status "statefulset/node-$primary" --timeout=120s
    Kube rollout status "statefulset/node-$standby" --timeout=120s
    Kube-Sync $primary $standby
    Kube-Wait {try{(Invoke-WebRequest http://127.0.0.1:18080/health/ready -TimeoutSec 4).StatusCode -eq 200}catch{$false}} 'routes after both copies return'
    $restorePair=$false
    Write-Output 'PASS: both copies unavailable returns 503; another shard stays readable; pair recovers.'
    Kube-Sql $standby 'SELECT pg_wal_replay_pause()'|Out-Null;$paused=$true
    # Fault injection creates durable primary WAL beyond the paused replay point.
    # This is a durability-counter write, not a bypass of any business mutation.
    Kube exec "node-$primary-0" -c db '--' env 'PGOPTIONS=-c synchronous_commit=local' psql -U erm -d erm -v ON_ERROR_STOP=1 -c "UPDATE requests SET version=version+1 WHERE id='$($before.id)'"
    $refused=$false
    try {& "$PSScriptRoot/k8s-failover.ps1" -Shard 0 -From $from} catch {if($_.Exception.Message -notlike '*Insufficient replay*'){throw};$refused=$true}
    if(!$refused -or (Kube-Sql $standby 'SELECT pg_is_in_recovery()') -ne 't'){throw 'Stale standby was promoted'}
    if((Kube get "statefulset/node-$primary" -o 'jsonpath={.spec.replicas}') -ne '0'){throw 'Old primary was not fenced'}
    Kube-Sql $standby 'SELECT pg_wal_replay_resume()'|Out-Null;$paused=$false
    $journal=Get-Content ".local/recovery/k8s-0-$from-v$($route.version).json" -Raw|ConvertFrom-Json
    Kube-Wait {(Kube-Sql $standby "SELECT pg_last_wal_replay_lsn()>='$($journal.lsn)'::pg_lsn") -eq 't'} 'catch-up to fenced primary WAL'
    & "$PSScriptRoot/k8s-failover.ps1" -Shard 0 -From $from
    & "$PSScriptRoot/k8s-rejoin.ps1" -Shard 0 -Target $from
    foreach($r in @($before,$during)){
        $got=Invoke-RestMethod "http://127.0.0.1:18080/requests/$($r.id)" -TimeoutSec 8
        if($got.createdAt -ne $r.createdAt){throw 'Acknowledged history changed'}
    }
    Write-Output 'PASS: fenced stale promotion refused; resumed operator sequence catches up, promotes and rejoins safely.'
} finally {
    if($paused){Kube-Sql $standby 'SELECT pg_wal_replay_resume()'|Out-Null}
    if($restorePair){Kube scale "statefulset/node-$primary" "statefulset/node-$standby" --replicas=1;Kube rollout status "statefulset/node-$standby" --timeout=120s;Kube-Sync $primary $standby}
    foreach($p in @($api,$control)){if($p -and !$p.HasExited){Stop-Process -Id $p.Id -ErrorAction SilentlyContinue}}
}
