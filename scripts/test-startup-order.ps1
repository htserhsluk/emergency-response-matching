$ErrorActionPreference='Stop'
$global:Day2Started=@{}
function docker {
    $global:LASTEXITCODE=0
    $s=$args -join ' '
    if ($s -like '*cat /control/routes.json*') {return '[{"nodeId":"node-0a"},{"nodeId":"node-1a"},{"nodeId":"node-2a"}]'}
    if ($s -like 'volume inspect*') {$global:LASTEXITCODE=1;return}
    if ($s -match 'up -d --wait db-(\d[ab])') {$global:Day2Started[$Matches[1]]=$true;return}
    if ($s -like '*SELECT pg_is_in_recovery()*') {if ($s -match 'db-\db ') {return 't'};return 'f'}
    if ($s -like '*SELECT count(*)*') {return '1'}
}
function wsl { $port=[int]$args[-1];$shard=($port-55432)/2;if(!$global:Day2Started["${shard}b"]) {throw 'Migration attempted before synchronous standby startup'};$global:LASTEXITCODE=0 }
function Invoke-WebRequest {return @{StatusCode=200}}
& ./scripts/start-lab.ps1
'PASS: cold startup starts synchronous standbys before migrations'
