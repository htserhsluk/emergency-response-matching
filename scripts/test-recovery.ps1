param([ValidateRange(0,2)][int]$Shard=0,[ValidateSet('a','b')][string]$From='a')
. "$PSScriptRoot/lab-common.ps1"
function Request-OnShard([string]$Tag) {
    $sha=[Security.Cryptography.SHA256]::Create()
    try {for ($n=0;$n -lt 10000;$n++) {
        $key="recovery-$Tag-$n"
        $hash=$sha.ComputeHash([Text.Encoding]::UTF8.GetBytes("erm:v1`0create-request`0$key"))
        $id=([BitConverter]::ToString($hash[0..15])).Replace('-','').ToLower()
        $routeHash=$sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($id));$mod=0
        foreach ($b in $routeHash[0..7]) {$mod=($mod*256+[int]$b)%3}
        if ($mod -eq $Shard) {
            return Invoke-RestMethod http://127.0.0.1:8080/requests -Method Post -Headers @{'Idempotency-Key'=$key} -ContentType application/json -Body '{"type":"WATER","location":{"latitude":12.9,"longitude":77.6},"details":"Recovery proof"}' -TimeoutSec 8
        }
    }} finally {$sha.Dispose()}
    throw 'No shard fixture found'
}
$tag=[Guid]::NewGuid().ToString('N')
$before=Request-OnShard "$tag-before"
& "$PSScriptRoot/failover-shard.ps1" -Shard $Shard -From $From
Wait-Condition {try {(Invoke-WebRequest http://127.0.0.1:8080/health/ready -TimeoutSec 3).StatusCode -eq 200}catch{$false}} 'promoted node activation'
$during=Request-OnShard "$tag-during"
$route=(Internal 'routes')[$Shard]
Compose restart coordinator
Wait-Condition {try {(Invoke-WebRequest http://127.0.0.1:8080/health/ready -TimeoutSec 3).StatusCode -eq 200}catch{$false}} 'coordinator restart'
if (((Internal 'routes')[$Shard]).nodeId -ne $route.nodeId) {throw 'Promoted route lost on restart'}
& "$PSScriptRoot/failover-shard.ps1" -Shard $Shard -From $From
& "$PSScriptRoot/rejoin-node.ps1" -Shard $Shard -Target $From
$count=Sql "$Shard$From" "SELECT count(*) FROM requests WHERE id IN ('$($before.id)','$($during.id)')"
if ($count -ne '2') {throw 'Rejoined standby missing pre-outage/outage data'}
foreach ($item in @($before,$during)) {
    $got=Invoke-RestMethod "http://127.0.0.1:8080/requests/$($item.id)" -TimeoutSec 8
    if ($got.id -ne $item.id -or $got.createdAt -ne $item.createdAt) {throw 'Recovered entity changed'}
}
Write-Output "PASS: shard $Shard promotion, coordinator restart, repeat activation, rejoin, and pre/outage data synchronization."
