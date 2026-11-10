param([Parameter(Mandatory)][ValidateRange(0,2)][int]$Shard,[Parameter(Mandatory)][ValidateSet('a','b')][string]$Target)
. "$PSScriptRoot/k8s-common.ps1"
$route=(Kube-Internal routes)[$Shard];$primary=$route.nodeId.Replace('node-','');$targetId="$Shard$Target"
if ($primary -eq $targetId -or $primary -notmatch "^${Shard}[ab]$"){throw 'Refusing current-primary or wrong-shard target'}
if ((Kube get "statefulset/node-$targetId" -o 'jsonpath={.spec.replicas}') -ne '0'){throw 'Target controller is not fenced'}
$pods=Kube get pods -l "node=node-$targetId" -o json|ConvertFrom-Json
if ($pods.items.Count -ne 0){throw 'Target still has a pod'}
$pvc=Kube get "pvc/data-node-$targetId-0" -o json|ConvertFrom-Json
if ($pvc.metadata.labels.node -ne "node-$targetId" -or $pvc.metadata.labels.app -ne 'erm'){throw 'Target PVC identity not verified'}
if ((Kube-Sql $primary 'SELECT pg_is_in_recovery()') -ne 'f'){throw 'Source is not primary'}
$system=Kube-Sql $primary 'SELECT system_identifier FROM pg_control_system()'
$jobName="rejoin-$targetId-v$($route.version)"
$previous=Kube get job $jobName --ignore-not-found -o json
if($previous){
    $previous=$previous|ConvertFrom-Json
    $failed=@($previous.status.conditions|Where-Object {$_.type -eq 'Failed' -and $_.status -eq 'True'}).Count -gt 0
    if($failed){
        # Foreground deletion waits for the old writer pod to disappear. Only the
        # Job/pods are replaced: the target PVC, archive and resume journal stay.
        Kube delete job $jobName --cascade=foreground --wait=true --timeout=90s
        $remaining=Kube get pods -l "job-name=$jobName" -o json|ConvertFrom-Json
        if($remaining.items.Count -ne 0){throw 'Previous recovery writer still exists; refusing another attempt'}
    }
}
$job=@{apiVersion='batch/v1';kind='Job';metadata=@{name=$jobName;namespace='erm'};spec=@{backoffLimit=0;template=@{spec=@{restartPolicy='Never';containers=@(@{name='rebuild';image='postgis/postgis:17-3.7@sha256:1f0d64ef12bfb344d3569ab6419aca183258b146951daec83f0585d01e6c9cc9';command=@('bash','/config/rejoin.sh');env=@(@{name='TARGET_ID';value=$targetId},@{name='PRIMARY_ID';value=$primary},@{name='EXPECTED_SYSTEM_ID';value=$system},@{name='PGPASSWORD';valueFrom=@{secretKeyRef=@{name='erm-config';key='ERM_REPLICATION_PASSWORD'}}});volumeMounts=@(@{name='target';mountPath='/target'},@{name='archive';mountPath='/archive'},@{name='scripts';mountPath='/config';readOnly=$true})});volumes=@(@{name='target';persistentVolumeClaim=@{claimName="data-node-$targetId-0"}},@{name='archive';persistentVolumeClaim=@{claimName='recovery-archive'}},@{name='scripts';configMap=@{name='postgres-scripts'}})}}}}
$job|ConvertTo-Json -Depth 20|& kubectl --context erm-lab -n erm apply -f -
if ($LASTEXITCODE){throw 'Recovery Job creation failed'}
Kube wait --for=condition=complete "job/$jobName" --timeout=180s
Kube logs "job/$jobName"
Kube scale "statefulset/node-$targetId" --replicas=1
Kube rollout status "statefulset/node-$targetId" --timeout=120s
if ((Kube-Sql $targetId 'SELECT pg_is_in_recovery()') -ne 't'){throw 'Recovered node is not standby'}
Kube-Sync $primary $targetId
Write-Output "Kubernetes paired mode restored; $primary remains primary."
