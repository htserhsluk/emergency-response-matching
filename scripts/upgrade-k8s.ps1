. "$PSScriptRoot/k8s-common.ps1"
$forward=$null
try{
    $forward=Start-KubeForward coordinator 18081 8081
    $routes=Kube-Internal routes
    # Quiesce the public publisher/API while applying the schema change.
    Kube scale deployment/api --replicas=0
    & kubectl --context erm-lab -n erm create secret generic erm-config --from-env-file=infra/compose/.env --dry-run=client -o yaml | & kubectl --context erm-lab -n erm apply -f -
    if ($LASTEXITCODE){throw 'Secret update failed'}
    & kubectl --context erm-lab -n erm create configmap postgres-scripts --from-file=infra/postgres --dry-run=client -o yaml | & kubectl --context erm-lab -n erm apply -f -
    if ($LASTEXITCODE){throw 'Config update failed'}
    foreach($route in $routes){
        $node=$route.nodeId.Replace('node-','')
        if ($node -notmatch '^[012][ab]$' -or (Kube-Sql $node 'SELECT pg_is_in_recovery()') -ne 'f'){throw 'Invalid active primary'}
        $manifest=(Get-Content infra/k8s/migration-job.yaml -Raw).Replace('__NODE__',$node).Replace('migrate-'+$node,'migrate-v4-'+$node)
        $manifest | & kubectl --context erm-lab -n erm apply -f -
        if ($LASTEXITCODE){throw 'Migration Job creation failed'}
        Kube wait --for=condition=complete "job/migrate-v4-$node" --timeout=120s
    }
    foreach($i in 0..2){foreach($side in @('a','b')){
        Kube rollout restart "statefulset/node-$i$side"
        Kube rollout status "statefulset/node-$i$side" --timeout=120s
    }}
    Kube apply -f infra/k8s/broker.yaml
    Kube rollout status statefulset/broker --timeout=180s
    Kube apply -f infra/k8s/apps.yaml
    Kube rollout restart deployment/coordinator deployment/api
    Kube rollout status deployment/coordinator --timeout=120s
    Kube rollout status deployment/api --timeout=120s
    Write-Output 'Updated images/schema and publisher deployed without resetting promoted routes.'
}finally{if($forward -and !$forward.HasExited){Stop-Process -Id $forward.Id -ErrorAction SilentlyContinue}}
