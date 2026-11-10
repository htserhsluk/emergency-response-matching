param([Parameter(Mandatory)][ValidateRange(0,2)][int]$Shard,[Parameter(Mandatory)][ValidateSet('a','b')][string]$Target)
. "$PSScriptRoot/lab-common.ps1"
$route=(Internal 'routes')[$Shard];$primary=$route.nodeId.Replace('node-','');$targetId="$Shard$Target"
if ($primary -eq $targetId -or !$primary.StartsWith("$Shard")) {throw 'Refusing current-primary or wrong-shard target'}
foreach ($service in @("node-$targetId","db-$targetId")) {
    $id=Compose ps -a -q $service
    if (!$id) {throw 'Missing target container'}
    $state=& docker inspect --format '{{.State.Running}} {{.HostConfig.RestartPolicy.Name}}' $id
    if ($LASTEXITCODE -or $state -ne 'false no') {throw 'Target must be stopped and restart disabled'}
}
$volume="erm-dev_db-$targetId"
$label=& docker volume inspect --format '{{index .Labels "com.docker.compose.project"}}' $volume
if ($LASTEXITCODE -or $label -ne 'erm-dev') {throw 'Target volume ownership not verified'}
if ((Sql $primary 'SELECT pg_is_in_recovery()') -ne 'f') {throw 'Source is not primary'}
$systemId=Sql $primary 'SELECT system_identifier FROM pg_control_system()'
$entry=Get-Content infra/compose/.env | Where-Object {$_ -like 'ERM_REPLICATION_PASSWORD=*'}
$env:PGPASSWORD=$entry.Substring('ERM_REPLICATION_PASSWORD='.Length)
try {
    $configPath=Join-Path $script:RepoRoot 'infra/postgres'
    & docker run --rm -e "NODE_ID=$primary" -v "erm-dev_db-${primary}:/source" -v erm-dev_recovery-archive:/archive -v "${configPath}:/config:ro" postgis/postgis:17-3.7@sha256:1f0d64ef12bfb344d3569ab6419aca183258b146951daec83f0585d01e6c9cc9 bash /config/archive-history.sh
    if ($LASTEXITCODE) {throw 'Could not archive historical backup directories'}
    & docker run --rm --network erm-dev_default -e PGPASSWORD -e "TARGET_ID=$targetId" -e "PRIMARY_ID=$primary" -e "EXPECTED_SYSTEM_ID=$systemId" -v "${volume}:/target" -v erm-dev_recovery-archive:/archive -v "${configPath}:/config:ro" postgis/postgis:17-3.7@sha256:1f0d64ef12bfb344d3569ab6419aca183258b146951daec83f0585d01e6c9cc9 bash /config/rejoin.sh
    if ($LASTEXITCODE) {throw 'Recovery refused or failed; original data preserved'}
} finally {Remove-Item Env:PGPASSWORD}
Compose up -d --wait "db-$targetId"
if ((Sql $targetId 'SELECT pg_is_in_recovery()') -ne 't') {throw 'Rejoined node is not a standby'}
Enable-Sync $primary $targetId
Compose up -d "node-$targetId"
Write-Output "Shard $Shard restored to paired synchronous mode; $primary remains primary."
