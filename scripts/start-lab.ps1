param([switch]$InitializeControl)
. "$PSScriptRoot/lab-common.ps1"
Compose build api coordinator node-0a matching
if ($InitializeControl) { Compose run --rm --no-deps --user 0 coordinator init-control /control/routes.json
    & docker run --rm -v erm-dev_coordinator-control:/control alpine:3.22@sha256:49340b346798875b5592e5d420241f2f9cfba5f47e62ebb1446a1b34734e898f chown -R 65532:65532 /control
    if ($LASTEXITCODE) {throw 'Control volume ownership failed'}
}
$routesRaw=& docker run --rm -v erm-dev_coordinator-control:/control:ro alpine:3.22@sha256:49340b346798875b5592e5d420241f2f9cfba5f47e62ebb1446a1b34734e898f cat /control/routes.json
if ($LASTEXITCODE) {throw 'Missing control state; explicit initialization required'}
$routes=$routesRaw | ConvertFrom-Json
if ($routes.Count -ne 3) {throw 'Invalid route count'}
foreach ($i in 0..2) {
    $primary=$routes[$i].nodeId.Replace('node-','')
    if ($primary -notmatch "^${i}[ab]$") {throw 'Invalid persisted primary identity'}
    Compose up -d --wait "db-$primary"
    if ((Sql $primary 'SELECT pg_is_in_recovery()') -ne 'f') {throw 'Persisted route points to a standby; inspect recovery state'}
    Compose exec -T "db-$primary" bash /config/replication-primary.sh
}
foreach ($i in 0..2) {
    $primary=$routes[$i].nodeId.Replace('node-','');$side=if ($primary.EndsWith('a')) {'b'} else {'a'};$standby="$i$side"
    # A stale former primary must be rebuilt while fenced, never restarted by this helper.
    $volume="erm-dev_db-$standby"
    & docker volume inspect $volume *> $null
    if ($LASTEXITCODE -eq 0) {
        & docker run --rm -v "${volume}:/target:ro" alpine:3.22@sha256:49340b346798875b5592e5d420241f2f9cfba5f47e62ebb1446a1b34734e898f sh -c 'test ! -s /target/PG_VERSION || test -f /target/standby.signal'
        if ($LASTEXITCODE) {throw "Former primary $standby is fenced; run rejoin-node before startup"}
    }
    Compose up -d --wait "db-$standby"
    if ((Sql $standby 'SELECT pg_is_in_recovery()') -ne 't') {throw 'Expected standby role'}
    Enable-Sync $primary $standby
    # Existing primaries retain synchronous settings across shutdown. Migrate only
    # once the standby can acknowledge WAL, including on a cold restart.
    $port=55432+2*$i
    if ($primary.EndsWith('b')) {$port++}
    & wsl -d Ubuntu --cd /mnt/c/Users/Kulshresth/Downloads/CCP/emergency-response-matching -- env PATH=/mnt/c/Users/Kulshresth/Downloads/CCP/.tools/go-linux/go/bin:/mnt/c/Users/Kulshresth/Downloads/CCP/.tools/bin:/usr/local/bin:/usr/bin:/bin bash scripts/migrate-local.sh $port
    if ($LASTEXITCODE) {throw "Migration failed on shard $i"}
}
Compose up -d node-0a node-0b node-1a node-1b node-2a node-2b coordinator broker api matching
Wait-Condition {try {(Invoke-WebRequest http://127.0.0.1:8080/health/ready -TimeoutSec 3).StatusCode -eq 200} catch {$false}} 'all shard routes ready'
Write-Output 'Three synchronous pairs and request/resource services ready.'
