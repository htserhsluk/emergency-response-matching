$ErrorActionPreference='Stop'
. "$PSScriptRoot/recovery-journal.ps1"
$root=Join-Path ([IO.Path]::GetTempPath()) "erm-journal-test-$([Guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory $root | Out-Null
$legacy=Join-Path $root 'k8s-0-a-v1.json'
'{"systemId":"111111","lsn":"0/100"}' | Set-Content $legacy
$other=Get-KubeJournalPath $root '222222' 0 a 1
if($other -eq $legacy){throw 'A different database reused the original recovery journal'}
if((Get-Content $legacy -Raw|ConvertFrom-Json).systemId -ne '111111'){throw 'Original journal overwritten'}
$resume=Get-KubeJournalPath $root '111111' 0 a 1
if($resume -ne $legacy){throw 'Matching legacy interrupted recovery was not preserved'}
$third=Get-KubeJournalPath $root '333333' 0 a 1
if($third -eq $other){throw 'Independent database identities collided'}
$refused=$false
try{Get-KubeJournalPath $root '../escape' 0 a 1 | Out-Null}catch{$refused=$true}
if(!$refused){throw 'Unsafe database identity accepted'}
'PASS: separate database journals, retained matching legacy resume, invalid identity refusal.'
