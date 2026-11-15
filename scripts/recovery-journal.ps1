function Get-KubeJournalPath([string]$Root,[string]$SystemId,[int]$Shard,[string]$From,[uint64]$Version) {
    if($SystemId -notmatch '^\d{1,20}$' -or $Shard -lt 0 -or $Shard -gt 2 -or $From -notin @('a','b') -or $Version -eq 0){
        throw 'Invalid recovery journal identity'
    }
    $name="k8s-$Shard-$From-v$Version.json"
    $legacy=Join-Path $Root $name
    # Preserve a valid interrupted recovery from earlier releases. A journal
    # from another installation must never be used to fence this database.
    if(Test-Path -LiteralPath $legacy){
        $saved=Get-Content -LiteralPath $legacy -Raw|ConvertFrom-Json
        if([string]$saved.systemId -eq $SystemId){return $legacy}
    }
    $directory=Join-Path $Root "system-$SystemId"
    New-Item -ItemType Directory -Force $directory | Out-Null
    Join-Path $directory $name
}
