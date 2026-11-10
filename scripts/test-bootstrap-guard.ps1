$ErrorActionPreference='Stop'
function kubectl {
    if (($args -join ' ') -match 'get (job|pvc)') { $global:LASTEXITCODE=0; return 'existing-installation' }
    throw 'UNSAFE_MUTATION_ATTEMPT'
}
try { & ./scripts/bootstrap-k8s.ps1; throw 'Expected bootstrap refusal' }
catch { if ($_.Exception.Message -notlike '*already initialized*') {throw}; 'PASS: existing installation refused before any mutation' }
