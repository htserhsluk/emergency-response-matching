. "$PSScriptRoot/k8s-common.ps1"
. "$PSScriptRoot/fixture-requests.ps1"
$api=$null;$control=$null;$workersStopped=$false
$tag=[Guid]::NewGuid().ToString('N')
$requests=[Collections.Generic.List[object]]::new()
function Add-Requests([int]$Count,[string]$Phase){
    foreach($i in 0..($Count-1)){$requests.Add((New-ShardRequest http://127.0.0.1:18080 ($i%3) "$tag-$Phase-$i"))}
}
function Wait-Assignments {
    Kube-Wait {
        foreach($r in $requests){try{$current=Invoke-RestMethod "http://127.0.0.1:18080/requests/$($r.id)" -TimeoutSec 8}catch{return $false};if($current.status -ne 'ASSIGNED'){return $false}}
        return $true
    } 'all queued requests assigned' 150
}
try {
    $api=Start-KubeForward api 18080 8080
    $control=Start-KubeForward coordinator 18081 8081
    Kube-Wait {try{(Invoke-WebRequest http://127.0.0.1:18080/health/ready -TimeoutSec 5).StatusCode -eq 200}catch{$false}} 'all shard routes ready'
    foreach($i in 0..39){
        $r=Invoke-RestMethod http://127.0.0.1:18080/resources -Method Post -Headers @{'Idempotency-Key'="supply-$tag-$i"} -ContentType application/json -Body '{"type":"WATER","location":{"latitude":12.9,"longitude":77.6},"availability":true}' -TimeoutSec 8
        if($r.status -ne 'AVAILABLE'){throw 'Supply creation failed'}
    }
    Kube scale deployment/matching --replicas=0
    $workersStopped=$true
    Kube wait --for=delete pod -l service=matching --timeout=90s
    Add-Requests 3 'single'
    foreach($r in $requests){$got=Invoke-RestMethod "http://127.0.0.1:18080/requests/$($r.id)";if($got.status -ne 'PENDING'){throw 'New work completed without its initial broker consumer'}}
    Kube scale deployment/matching --replicas=1
    $workersStopped=$false
    Kube rollout status deployment/matching --timeout=120s
    Wait-Assignments
    Add-Requests 9 'scale'
    Kube scale deployment/matching --replicas=2
    Kube rollout status deployment/matching --timeout=120s
    Wait-Assignments
    $pods=(Kube get pods -l service=matching -o json|ConvertFrom-Json).items
    if($pods.Count -ne 2){throw 'Expected two matching workers'}
    $old=$pods[0].metadata.uid
    Kube delete pod $pods[0].metadata.name --wait=true
    Add-Requests 3 'replacement'
    Kube rollout status deployment/matching --timeout=120s
    $newPods=(Kube get pods -l service=matching -o json|ConvertFrom-Json).items
    if($newPods.Count -ne 2 -or $old -in $newPods.metadata.uid){throw 'Worker pod was not replaced'}
    Wait-Assignments
    Kube delete pod broker-0 --wait=true
    Add-Requests 3 'broker-restart'
    Kube rollout status statefulset/broker --timeout=180s
    Wait-Assignments
    Stop-Process -Id $control.Id -ErrorAction SilentlyContinue
    Kube rollout restart deployment/coordinator
    Kube rollout status deployment/coordinator --timeout=120s
    $control=Start-KubeForward coordinator 18081 8081
    Kube-Wait {try{(Invoke-WebRequest http://127.0.0.1:18080/health/ready -TimeoutSec 5).StatusCode -eq 200}catch{$false}} 'routes after coordinator restart'
    Wait-Assignments
    $owners=@{}
    foreach($r in $requests){
        $got=Invoke-RestMethod "http://127.0.0.1:18080/requests/$($r.id)" -TimeoutSec 8
        if(!$got.assignedResourceId -or $owners.ContainsKey($got.assignedResourceId)){throw 'Resource allocated more than once'}
        $owners[$got.assignedResourceId]=$r.id
    }
    $first=$requests[0]
    $complete=Invoke-RestMethod "http://127.0.0.1:18080/requests/$($first.id)/status" -Method Put -Headers @{'Idempotency-Key'="complete-$tag"} -ContentType application/json -Body '{"status":"COMPLETED"}' -TimeoutSec 8
    if($complete.status -ne 'COMPLETED'){throw 'Completion failed'}
    $resource=Kube-Internal "data/resource:$($complete.assignedResourceId)"
    if($resource.availability -or $resource.status -ne 'ALLOCATED'){throw 'Completion incorrectly released resource'}
    Kube get 'deployments,pods'
    Write-Output "PASS: $($requests.Count) unique assignments, one-to-two worker scaling, worker replacement, broker reconnect, coordinator restart and completion protection."
}finally{
    if($workersStopped){Kube scale deployment/matching --replicas=1}
    foreach($p in @($api,$control)){if($p -and !$p.HasExited){Stop-Process -Id $p.Id -ErrorAction SilentlyContinue}}
}
