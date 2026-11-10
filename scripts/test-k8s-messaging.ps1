. "$PSScriptRoot/k8s-common.ps1"
. "$PSScriptRoot/fixture-requests.ps1"
$api=$null;$coordinator=$null;$brokerStopped=$false
$tag=[Guid]::NewGuid().ToString('N')
try {
    $api=Start-KubeForward api 18080 8080
    $coordinator=Start-KubeForward coordinator 18081 8081
    $routes=Kube-Internal routes
    $requests=@()
    foreach($shard in 0..2) {
        $request=New-ShardRequest http://127.0.0.1:18080 $shard "messaging-$tag-$shard"
        $requests+=$request
        $primary=$routes[$shard].nodeId.Replace('node-','')
        $standby="$shard"+$(if($primary.EndsWith('a')){'b'}else{'a'})
        if((Kube-Sql $primary "SELECT count(*) FROM requests WHERE id='$($request.id)'") -ne '1'){throw 'Request missing from expected shard'}
        if((Kube-Sql $standby "SELECT count(*) FROM requests WHERE id='$($request.id)'") -ne '1'){throw 'Request missing from synchronous replica'}
        Kube-Wait { (Kube-Sql $primary "SELECT count(*) FROM outbox_events WHERE request_id='$($request.id)' AND sent_at IS NOT NULL") -eq '1' } 'confirmed outbox publication' 120
    }
    $resource=Invoke-RestMethod http://127.0.0.1:18080/resources -Method Post -Headers @{'Idempotency-Key'="resource-$tag"} -ContentType application/json -Body '{"type":"WATER","location":{"latitude":12.9,"longitude":77.6},"availability":true}' -TimeoutSec 8
    if($resource.status -ne 'AVAILABLE'){throw 'Resource registration failed'}
    # Stop only this project's broker, prove database acceptance, then recover it.
    Kube scale statefulset/broker --replicas=0
    $brokerStopped=$true
    Kube wait --for=delete pod/broker-0 --timeout=90s
    $outage=New-ShardRequest http://127.0.0.1:18080 0 "broker-outage-$tag"
    $primary=$routes[0].nodeId.Replace('node-','')
    if((Kube-Sql $primary "SELECT count(*) FROM outbox_events WHERE request_id='$($outage.id)' AND sent_at IS NULL") -ne '1'){throw 'Broker outage lost the pending delivery'}
    Kube scale statefulset/broker --replicas=1
    Kube rollout status statefulset/broker --timeout=180s
    $brokerStopped=$false
    Kube-Wait { (Kube-Sql $primary "SELECT count(*) FROM outbox_events WHERE request_id='$($outage.id)' AND sent_at IS NOT NULL") -eq '1' } 'publication after broker recovery' 150
    $apiManifest=(Kube get deployment api -o json)|ConvertFrom-Json
    $job="inspect-$($tag.Substring(0,12))"
    $manifest=@{apiVersion='batch/v1';kind='Job';metadata=@{name=$job;namespace='erm'};spec=@{backoffLimit=0;template=@{spec=@{restartPolicy='Never';containers=@(@{name='inspect';image='erm-matching:day2';imagePullPolicy='Never';args=@('--inspect-one');env=$apiManifest.spec.template.spec.containers[0].env})}}}}
    $manifest|ConvertTo-Json -Depth 20|& kubectl --context erm-lab -n erm apply -f -
    if($LASTEXITCODE){throw 'Inspection Job creation failed'}
    Kube wait --for=condition=complete "job/$job" --timeout=60s
    $inspection=(Kube logs "job/$job")|ConvertFrom-Json
    if($inspection.request.status -ne 'PENDING' -or $inspection.candidates.Count -lt 1){throw 'Consumer did not retrieve request and candidates'}
    $after=Invoke-RestMethod "http://127.0.0.1:18080/requests/$($inspection.request.id)" -TimeoutSec 8
    if($after.status -ne 'PENDING'){throw 'Diagnostic consumer incorrectly completed request'}
    Kube exec broker-0 '--' rabbitmqctl list_queues name messages_ready messages_unacknowledged
    Write-Output 'PASS: three-shard storage/replicas, confirmed publication, broker outage recovery, consumer lookup/search and requeue.'
} finally {
    if($brokerStopped){Kube scale statefulset/broker --replicas=1}
    foreach($p in @($api,$coordinator)){if($p -and !$p.HasExited){Stop-Process -Id $p.Id -ErrorAction SilentlyContinue}}
}
