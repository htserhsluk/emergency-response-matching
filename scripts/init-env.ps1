$ErrorActionPreference = 'Stop'
$target = Join-Path $PSScriptRoot '../infra/compose/.env'
if (Test-Path -LiteralPath $target) { throw 'Local environment already exists; refusing to replace credentials' }
$lines = @('ERM_DB_PASSWORD', 'ERM_INTERNAL_SECRET', 'ERM_REPLICATION_PASSWORD', 'ERM_BROKER_PASSWORD') | ForEach-Object {
    $_ + '=' + [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(24))
}
[IO.File]::WriteAllText($target, ($lines -join "`n") + "`n")
Write-Output 'Created local-only environment file.'
