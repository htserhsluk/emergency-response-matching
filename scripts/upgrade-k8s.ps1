. "$PSScriptRoot/k8s-common.ps1"
$forward=$null
try{
    $forward=Start-KubeForward coordinator 18081 8081
    $routes=Kube-Internal routes
    $version=[long]((Get-ChildItem migrations/*.sql|Sort-Object Name|Select-Object -Last 1).BaseName.Split('_')[0])
    $worker=Kube get deployment matching --ignore-not-found -o name
    if($worker){Kube scale deployment/matching --replicas=0;Kube wait --for=delete pod -l service=matching --timeout=90s}
    # Quiesce the public publisher/API while applying the schema change.
    Kube scale deployment/api --replicas=0
    Kube scale deployment/coordinator --replicas=0
    Kube wait --for=delete pod -l service=coordinator --timeout=90s
    & kubectl --context erm-lab -n erm create secret generic erm-config --from-env-file=infra/compose/.env --dry-run=client -o yaml | & kubectl --context erm-lab -n erm apply -f -
    if ($LASTEXITCODE){throw 'Secret update failed'}
    & kubectl --context erm-lab -n erm create configmap postgres-scripts --from-file=infra/postgres --dry-run=client -o yaml | & kubectl --context erm-lab -n erm apply -f -
    if ($LASTEXITCODE){throw 'Config update failed'}
    foreach($route in $routes){
        $node=$route.nodeId.Replace('node-','')
        if ($node -notmatch '^[012][ab]$' -or (Kube-Sql $node 'SELECT pg_is_in_recovery()') -ne 'f'){throw 'Invalid active primary'}
        $manifest=(Get-Content infra/k8s/migration-job.yaml -Raw).Replace('__NODE__',$node).Replace('migrate-'+$node,"migrate-v$version-$node")
        $manifest | & kubectl --context erm-lab -n erm apply -f -
        if ($LASTEXITCODE){throw 'Migration Job creation failed'}
        Kube wait --for=condition=complete "job/migrate-v$version-$node" --timeout=120s
    }
    foreach($i in 0..2){foreach($side in @('a','b')){
        Kube set image "statefulset/node-$i$side" agent=erm-storage-node:day3
        Kube rollout restart "statefulset/node-$i$side"
        Kube rollout status "statefulset/node-$i$side" --timeout=120s
    }}
    Kube apply -f infra/k8s/broker.yaml
    Kube rollout status statefulset/broker --timeout=180s
    Kube apply -f infra/k8s/apps.yaml
    Kube rollout restart deployment/coordinator deployment/api
    Kube rollout status deployment/coordinator --timeout=120s
    Kube rollout status deployment/api --timeout=120s
    Kube apply -f infra/k8s/matching.yaml
    Kube rollout status deployment/matching --timeout=120s
    Write-Output 'Updated images/schema and publisher deployed without resetting promoted routes.'
}finally{if($forward -and !$forward.HasExited){Stop-Process -Id $forward.Id -ErrorAction SilentlyContinue}}
