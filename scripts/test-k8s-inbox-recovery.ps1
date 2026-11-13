. "$PSScriptRoot/k8s-common.ps1"
. "$PSScriptRoot/fixture-requests.ps1"
$api=$null;$control=$null
try{
 $api=Start-KubeForward api 18080 8080;$control=Start-KubeForward coordinator 18081 8081
 $base='http://127.0.0.1:18080';$tag=[Guid]::NewGuid().ToString('N')
 Invoke-RestMethod "$base/resources" -Method Post -Headers @{'Idempotency-Key'="inbox-resource-$tag"} -ContentType application/json -Body '{"type":"WATER","location":{"latitude":12.9,"longitude":77.6},"availability":true}' -TimeoutSec 8 | Out-Null
 $request=New-ShardRequest $base 0 "inbox-$tag"
 Kube-Wait {(Invoke-RestMethod "$base/requests/$($request.id)" -TimeoutSec 8).status -eq 'ASSIGNED'} 'inbox fixture assignment'
 $assigned=Invoke-RestMethod "$base/requests/$($request.id)" -TimeoutSec 8
 if(!$assigned.assignment -or !$assigned.assignment.location){throw 'Saved assignment summary missing'}
 $list=Invoke-RestMethod "$base/requests/$($request.id)/notifications" -TimeoutSec 8
 if($list.items.Count -ne 1 -or $list.items[0].resourceId -ne $assigned.assignedResourceId){throw 'Assignment inbox not atomic/unique'}
 $notification=$list.items[0]
 $read=Invoke-RestMethod "$base/requests/$($request.id)/notifications/$($notification.id)/read" -Method Put -Headers @{'Idempotency-Key'="inbox-read-$tag"} -ContentType application/json -Body '{}' -TimeoutSec 8
 if(!$read.readAt){throw 'Read state not persisted'}
 Stop-Process -Id $api.Id -ErrorAction SilentlyContinue
 Kube rollout restart deployment/api;Kube rollout status deployment/api --timeout=120s
 $api=Start-KubeForward api 18080 8080
 $list=Invoke-RestMethod "$base/requests/$($request.id)/notifications" -TimeoutSec 8
 if($list.items.Count -ne 1 -or $list.items[0].readAt -ne $read.readAt){throw 'Read state lost after API restart'}
 $route=(Kube-Internal routes)[0];$from=$route.nodeId.Substring($route.nodeId.Length-1)
 & "$PSScriptRoot/k8s-failover.ps1" -Shard 0 -From $from
 & "$PSScriptRoot/k8s-rejoin.ps1" -Shard 0 -Target $from
 $list=Invoke-RestMethod "$base/requests/$($request.id)/notifications" -TimeoutSec 8
 if($list.items.Count -ne 1 -or $list.items[0].readAt -ne $read.readAt){throw 'Read state lost/duplicated after shard promotion'}
 if((Kube-Sql "0$from" "SELECT count(*) FROM notifications WHERE id='$($notification.id)' AND read_at IS NOT NULL") -ne '1'){throw 'Recovered replica lacks read state'}
 Invoke-RestMethod "$base/match/$($request.id)" -Method Post -Headers @{'Idempotency-Key'="inbox-duplicate-$tag"} -ContentType application/json -Body '{}' -TimeoutSec 8|Out-Null
 $list=Invoke-RestMethod "$base/requests/$($request.id)/notifications" -TimeoutSec 8
 if($list.items.Count -ne 1){throw 'Repeated matching duplicated inbox'}
 Write-Output "PASS: assignment summary, atomic unique inbox, read state across API restart, request-shard promotion and replica rebuild. Request=$($request.id) Notification=$($notification.id)"
}finally{foreach($p in @($api,$control)){if($p -and !$p.HasExited){Stop-Process -Id $p.Id -ErrorAction SilentlyContinue}}}
