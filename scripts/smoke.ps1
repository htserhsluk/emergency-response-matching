$ErrorActionPreference = 'Stop'
$base = 'http://127.0.0.1:8080'
$key = 'day1-smoke-key-0005'
$expectedID = 'a421d834d1d485e1d42021047b45207a'
$body = '{"type":"WATER","location":{"latitude":12.97,"longitude":77.59},"details":"Day 1 persistence fixture"}'
$response = Invoke-WebRequest "$base/requests" -Method Post -ContentType application/json -Headers @{'Idempotency-Key'=$key} -Body $body -SkipHttpErrorCheck
if ($response.StatusCode -ne 201) { throw "Create returned $($response.StatusCode): $($response.Content)" }
$created = $response.Content | ConvertFrom-Json
if ($created.id -ne $expectedID -or $created.status -ne 'PENDING') { throw 'Unexpected persisted identity/status' }
$read = Invoke-RestMethod "$base/requests/$expectedID"
if ($read.details -ne 'Day 1 persistence fixture') { throw 'Retrieval mismatch' }
$replay = Invoke-WebRequest "$base/requests" -Method Post -ContentType application/json -Headers @{'Idempotency-Key'=$key} -Body $body
if (($replay.Content | ConvertFrom-Json).createdAt -ne $created.createdAt) { throw 'Replay changed creation time' }
$conflict = Invoke-WebRequest "$base/requests" -Method Post -ContentType application/json -Headers @{'Idempotency-Key'=$key} -Body ($body.Replace('Day 1 persistence fixture','Changed payload')) -SkipHttpErrorCheck
if ($conflict.StatusCode -ne 409) { throw 'Changed payload was not rejected' }
$missingRoute = Invoke-WebRequest "$base/requests/00000000000000000000000000000000" -SkipHttpErrorCheck
if ($missingRoute.StatusCode -ne 503) { throw 'Unconfigured shard must fail closed' }
Write-Output "PASS: create, retrieve, stable replay, conflict and missing-shard exclusion. Persistent ID: $expectedID"
