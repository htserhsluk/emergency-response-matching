$ErrorActionPreference='Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
function Kube {
    & kubectl --context erm-lab -n erm @args
    if ($LASTEXITCODE) {throw "Kubernetes operation failed (exit $LASTEXITCODE)"}
}
function Kube-Sql([string]$Node,[string]$Statement) {
    $result=Kube exec "node-$Node-0" -c db '--' env 'PGOPTIONS=-c statement_timeout=15000' psql -U erm -d erm -At -v ON_ERROR_STOP=1 -c $Statement
    return ($result -join "`n").Trim()
}
function Kube-Wait([scriptblock]$Check,[string]$Description,[int]$Seconds=90){
    $deadline=(Get-Date).AddSeconds($Seconds)
    do {if (& $Check){return};Start-Sleep -Seconds 1} while ((Get-Date)-lt $deadline)
    throw "Timed out: $Description"
}
function Kube-Internal([string]$Path,[string]$Method='GET',$Body=$null){
    $entry=Get-Content infra/compose/.env | Where-Object {$_ -like 'ERM_INTERNAL_SECRET=*'}
    $params=@{Uri="http://127.0.0.1:18081/internal/$Path";Method=$Method;Headers=@{'X-Internal-Secret'=$entry.Substring('ERM_INTERNAL_SECRET='.Length)};TimeoutSec=8}
    if ($null -ne $Body){$params.ContentType='application/json';$params.Body=$Body|ConvertTo-Json -Compress}
    Invoke-RestMethod @params
}
function Start-KubeForward([string]$Service,[int]$LocalPort,[int]$RemotePort){
    New-Item -ItemType Directory -Force .local | Out-Null
    if($Service -eq 'api'){
        # Target the Service IP instead of binding the listener to one API pod.
        $address=Kube get service api -o 'jsonpath={.spec.clusterIP}'
        $binding=docker port erm-lab 22/tcp
        if($LASTEXITCODE -or $binding -notmatch '^127\.0\.0\.1:(\d+)$'){throw 'Missing loopback SSH port on the erm-lab Docker node'}
        $sshPort=$Matches[1]
        $key=minikube -p erm-lab ssh-key
        if($LASTEXITCODE -or !(Test-Path -LiteralPath $key)){throw 'Cannot locate the erm-lab SSH key'}
        $publicKey=docker exec erm-lab cat /etc/ssh/ssh_host_ed25519_key.pub
        if($LASTEXITCODE -or $publicKey -notmatch '^ssh-ed25519 '){throw 'Cannot verify the erm-lab SSH host key'}
        $hostsFile=".local/forward-$LocalPort.known_hosts"
        "[127.0.0.1]:$sshPort $publicKey" | Set-Content -LiteralPath $hostsFile -Encoding ascii
        $arguments=@('-N','-T','-o','BatchMode=yes','-o','ExitOnForwardFailure=yes','-o','StrictHostKeyChecking=yes','-o',"UserKnownHostsFile=$hostsFile",'-o','ServerAliveInterval=15','-p',$sshPort,'-i',('"{0}"' -f $key),'-L',"127.0.0.1:${LocalPort}:${address}:$RemotePort",'docker@127.0.0.1')
        $proc=Start-Process (Get-Command ssh).Source -ArgumentList $arguments -WindowStyle Hidden -PassThru -RedirectStandardError ".local/forward-$LocalPort.err"
    }else{
        $proc=Start-Process (Get-Command kubectl).Source -ArgumentList @('--context','erm-lab','-n','erm','port-forward',"service/$Service","${LocalPort}:$RemotePort") -WindowStyle Hidden -PassThru -RedirectStandardOutput ".local/forward-$LocalPort.log" -RedirectStandardError ".local/forward-$LocalPort.err"
    }
    try{
        Kube-Wait {if ($proc.HasExited){throw "Local connection exited. See .local/forward-$LocalPort.err"};try{(Invoke-WebRequest "http://127.0.0.1:$LocalPort/health/live" -TimeoutSec 2).StatusCode -eq 200}catch{$false}} 'local service connection'
        if($proc.HasExited){throw 'Local service connection exited during startup'}
        return $proc
    }catch{
        if(!$proc.HasExited){$proc.Kill()}
        throw
    }
}
function Kube-Sync([string]$Primary,[string]$Standby){
    Kube-Wait { (Kube-Sql $Primary "SELECT count(*) FROM pg_stat_replication WHERE application_name='node-$Standby' AND state='streaming'") -eq '1' } 'streaming replica'
    Kube-Sql $Primary "ALTER SYSTEM SET synchronous_standby_names = 'FIRST 1 (`"node-$Standby`")'" | Out-Null
    Kube-Sql $Primary 'ALTER SYSTEM SET synchronous_commit = remote_apply' | Out-Null
    Kube-Sql $Primary 'SELECT pg_reload_conf()' | Out-Null
    Kube-Wait { (Kube-Sql $Primary "SELECT count(*) FROM pg_stat_replication WHERE application_name='node-$Standby' AND sync_state='sync'") -eq '1' } 'synchronous replica'
}
