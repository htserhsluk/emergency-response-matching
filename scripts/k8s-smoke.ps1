$ErrorActionPreference = 'Stop'
kubectl --context erm-lab -n erm-smoke rollout status deployment/api-health --timeout=120s
if ($LASTEXITCODE) { throw 'Smoke deployment failed' }
$forward = Start-Process -FilePath (Get-Command kubectl).Source -ArgumentList @('--context','erm-lab','-n','erm-smoke','port-forward','service/api-health','18080:8080') -WindowStyle Hidden -PassThru
try {
    $ready = $false
    for ($attempt=0; $attempt -lt 30; $attempt++) {
        try {
            $result = Invoke-RestMethod 'http://127.0.0.1:18080/health/live' -TimeoutSec 2
            if ($result.status -eq 'live') { $ready=$true; break }
        } catch { Start-Sleep -Milliseconds 300 }
    }
    if (-not $ready) { throw 'API health could not be reached through Kubernetes Service' }
    Write-Output 'PASS: real Go API image reached through Kubernetes Deployment and Service (health-only smoke).'
} finally {
    if (-not $forward.HasExited) { Stop-Process -Id $forward.Id }
}
