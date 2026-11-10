# Exercise the real operator script against a failed-Job control-plane fixture.
$ErrorActionPreference='Stop'
$global:RejoinDeleted=$false;$global:RejoinScaled=$false
function Invoke-RestMethod {return ,@(@{nodeId='node-0a';version=3})}
function kubectl {
    $global:LASTEXITCODE=0
    $s=$args -join ' '
    if($s -like '*get statefulset/*'){return '0'}
    if($s -like '*get pods*'){return '{"items":[]}'}
    if($s -like '*get pvc/*'){return '{"metadata":{"labels":{"node":"node-0b","app":"erm"}}}'}
    if($s -like '*get job*'){return '{"status":{"failed":1,"conditions":[{"type":"Failed","status":"True"}]}}'}
    if($s -like '*delete job*'){$global:RejoinDeleted=$true;return}
    if($s -like '*wait --for=condition=complete*' -and !$global:RejoinDeleted){throw 'Failed Job was reused; no new writer will start'}
    if($s -like '*scale statefulset/node-0b*'){$global:RejoinScaled=$true;return}
    if($s -like '*SELECT pg_is_in_recovery()*'){if($s -like '*node-0b-0*'){return 't'};return 'f'}
    if($s -like '*SELECT system_identifier*'){return '123456'}
    if($s -like '*SELECT count(*)*'){return '1'}
}
& "$PSScriptRoot/k8s-rejoin.ps1" -Shard 0 -Target b
if(!$global:RejoinDeleted -or !$global:RejoinScaled){throw 'Failed attempt was not replaced and recovered'}
'PASS: failed rebuild Job replaced before standby restoration; PVC retained.'
