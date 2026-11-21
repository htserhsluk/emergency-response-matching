. "$PSScriptRoot/k8s-common.ps1"
$connection=Start-KubeForward api 18080 8080
try{
    Write-Host 'Dashboard available at http://localhost:18080. Press Ctrl+C to close.'
    while(!$connection.HasExited){Start-Sleep -Milliseconds 500}
    if($connection.ExitCode){throw "Dashboard connection exited. See .local/forward-18080.err"}
}finally{
    if(!$connection.HasExited){$connection.Kill()}
}
