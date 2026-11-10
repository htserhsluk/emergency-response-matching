. "$PSScriptRoot/k8s-common.ps1"
# Initial installation only. Subsequent starts preserve saved StatefulSet replica counts.
$existing = & kubectl --context erm-lab -n erm get pvc coordinator-control --ignore-not-found -o name
if ($LASTEXITCODE) {throw 'Cannot establish whether this cluster is initialized; refusing bootstrap'}
if ($existing) {throw 'Cluster already initialized; use upgrade-k8s.ps1 or inspect an interrupted initial installation'}
& kubectl --context erm-lab create namespace erm --dry-run=client -o yaml | & kubectl --context erm-lab apply -f -
if ($LASTEXITCODE){throw 'Namespace failed'}
& kubectl --context erm-lab -n erm create secret generic erm-config --from-env-file=infra/compose/.env --dry-run=client -o yaml | & kubectl --context erm-lab -n erm apply -f -
if ($LASTEXITCODE){throw 'Secret failed'}
& kubectl --context erm-lab -n erm create configmap postgres-scripts --from-file=infra/postgres --dry-run=client -o yaml | & kubectl --context erm-lab -n erm apply -f -
if ($LASTEXITCODE){throw 'Postgres config failed'}
Kube apply -f infra/k8s/apps.yaml
Kube apply -f infra/k8s/initialize-control.yaml
Kube wait --for=condition=complete job/initialize-control --timeout=90s
Kube apply -f infra/k8s/storage.yaml
foreach ($i in 0..2){
    Kube scale "statefulset/node-${i}a" --replicas=1
    Kube rollout status "statefulset/node-${i}a" --timeout=180s
    Kube exec "node-${i}a-0" -c db '--' bash /config/replication-primary.sh
    $manifest=(Get-Content infra/k8s/migration-job.yaml -Raw).Replace('__NODE__',"${i}a")
    $manifest | & kubectl --context erm-lab -n erm apply -f -
    if ($LASTEXITCODE){throw 'Migration Job creation failed'}
    Kube wait --for=condition=complete "job/migrate-${i}a" --timeout=120s
    Kube scale "statefulset/node-${i}b" --replicas=1
    Kube rollout status "statefulset/node-${i}b" --timeout=180s
    Kube-Sync "${i}a" "${i}b"
}
Kube apply -f infra/k8s/broker.yaml
Kube rollout status statefulset/broker --timeout=180s
Kube rollout status deployment/coordinator --timeout=120s
Kube rollout status deployment/api --timeout=120s
Kube get pods,pvc
Write-Output 'Kubernetes: three synchronous storage pairs and API/coordinator ready.'
