param([ValidateRange(0,2)][int]$Shard=0,[ValidateSet('a','b')][string]$From='a',[switch]$TestInterruptedRejoin)
. "$PSScriptRoot/k8s-common.ps1"
. "$PSScriptRoot/fixture-requests.ps1"
$apiForward=$null;$controlForward=$null
try{
    $apiForward=Start-KubeForward api 18080 8080
    $controlForward=Start-KubeForward coordinator 18081 8081
    $tag=[Guid]::NewGuid().ToString('N');$before=New-ShardRequest 'http://127.0.0.1:18080' $Shard "$tag-before"
    & "$PSScriptRoot/k8s-failover.ps1" -Shard $Shard -From $From
    Kube-Wait {try{(Invoke-WebRequest http://127.0.0.1:18080/health/ready -TimeoutSec 3).StatusCode -eq 200}catch{$false}} 'new route activation'
    $during=New-ShardRequest 'http://127.0.0.1:18080' $Shard "$tag-during"
    $route=(Kube-Internal routes)[$Shard]
    Stop-Process -Id $controlForward.Id -ErrorAction SilentlyContinue
    Kube rollout restart deployment/coordinator
    Kube rollout status deployment/coordinator --timeout=120s
    $controlForward=Start-KubeForward coordinator 18081 8081
    if (((Kube-Internal routes)[$Shard]).nodeId -ne $route.nodeId){throw 'Route lost on coordinator restart'}
    & "$PSScriptRoot/k8s-failover.ps1" -Shard $Shard -From $From
    if($TestInterruptedRejoin){
        # A terminated backup attempt leaves partial incoming files. The operator
        # must replace its failed Job and preserve those files before retrying.
        $jobName="rejoin-$Shard$From-v$($route.version)"
        $job=@{apiVersion='batch/v1';kind='Job';metadata=@{name=$jobName;namespace='erm'};spec=@{backoffLimit=0;template=@{spec=@{restartPolicy='Never';containers=@(@{name='rebuild';image='alpine:3.22@sha256:49340b346798875b5592e5d420241f2f9cfba5f47e62ebb1446a1b34734e898f';command=@('sh','-c','mkdir /target/.rejoin-new && echo interrupted > /target/.rejoin-new/interrupted-marker; exit 1');volumeMounts=@(@{name='target';mountPath='/target'})});volumes=@(@{name='target';persistentVolumeClaim=@{claimName="data-node-$Shard$From-0"}})}}}}
        $job|ConvertTo-Json -Depth 20|& kubectl --context erm-lab -n erm apply -f -
        if($LASTEXITCODE){throw 'Failed-attempt fixture creation failed'}
        Kube wait --for=condition=Failed "job/$jobName" --timeout=90s
    }
    & "$PSScriptRoot/k8s-rejoin.ps1" -Shard $Shard -Target $From
    if ((Kube-Sql "$Shard$From" "SELECT count(*) FROM requests WHERE id IN ('$($before.id)','$($during.id)')") -ne '2'){throw 'Recovered PVC lacks outage-period data'}
    foreach($r in @($before,$during)){$got=Invoke-RestMethod "http://127.0.0.1:18080/requests/$($r.id)" -TimeoutSec 8;if($got.id -ne $r.id -or $got.createdAt -ne $r.createdAt){throw 'Entity changed across recovery'}}
    Write-Output 'PASS: Kubernetes fencing, promotion, persistent route, outage writes, recovery Job, and replica synchronization.'
}finally{
    foreach($proc in @($apiForward,$controlForward)){if($proc -and !$proc.HasExited){Stop-Process -Id $proc.Id -ErrorAction SilentlyContinue}}
}
