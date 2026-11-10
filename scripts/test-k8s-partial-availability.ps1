. "$PSScriptRoot/k8s-common.ps1"
. "$PSScriptRoot/fixture-requests.ps1"
$api=$null;$control=$null;$stopped=$false
$probe='availability-'+[Guid]::NewGuid().ToString('N').Substring(0,12)
try {
    $api=Start-KubeForward api 18080 8080
    $control=Start-KubeForward coordinator 18081 8081
    $request=New-ShardRequest http://127.0.0.1:18080 1 $probe
    $route=(Kube-Internal routes)[0]
    Kube run $probe --image=alpine:3.22@sha256:49340b346798875b5592e5d420241f2f9cfba5f47e62ebb1446a1b34734e898f --restart=Never '--' sleep 300
    Kube wait --for=condition=Ready "pod/$probe" --timeout=90s
    Kube scale "statefulset/$($route.nodeId)" --replicas=0
    $stopped=$true
    Kube wait --for=delete "pod/$($route.nodeId)-0" --timeout=90s
    # Let heartbeat expiry and Kubernetes readiness selection observe the outage.
    Start-Sleep -Seconds 15
    $result=(Kube exec $probe '--' wget -q -T 8 -O - "http://api:8080/requests/$($request.id)")|ConvertFrom-Json
    if($result.id -ne $request.id){throw 'Healthy shard request unavailable through API Service'}
    Write-Output 'PASS: healthy shard remains reachable through actual Kubernetes Service during another shard outage.'
}finally{
    if($stopped){Kube scale "statefulset/$($route.nodeId)" --replicas=1;Kube rollout status "statefulset/$($route.nodeId)" --timeout=120s}
    Kube delete pod $probe --ignore-not-found --wait=true
    foreach($p in @($api,$control)){if($p -and !$p.HasExited){Stop-Process -Id $p.Id -ErrorAction SilentlyContinue}}
}
