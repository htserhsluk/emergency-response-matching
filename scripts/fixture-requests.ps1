function New-ShardRequest([string]$Base,[int]$Shard,[string]$Tag){
    $sha=[Security.Cryptography.SHA256]::Create()
    try{for($n=0;$n -lt 10000;$n++){
        $key="fixture-$Tag-$n";$hash=$sha.ComputeHash([Text.Encoding]::UTF8.GetBytes("erm:v1`0create-request`0$key"));$id=([BitConverter]::ToString($hash[0..15])).Replace('-','').ToLower()
        $routeHash=$sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($id));$mod=0;foreach($b in $routeHash[0..7]){$mod=($mod*256+[int]$b)%3}
        if($mod -eq $Shard){return Invoke-RestMethod "$Base/requests" -Method Post -Headers @{'Idempotency-Key'=$key} -ContentType application/json -Body '{"type":"WATER","location":{"latitude":12.9,"longitude":77.6},"details":"Storage recovery proof"}' -TimeoutSec 8}
    }}finally{$sha.Dispose()};throw 'No fixture key found'
}
