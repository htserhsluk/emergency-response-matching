. "$PSScriptRoot/k8s-common.ps1"
$forward=$null
try{
 $forward=Start-KubeForward api 18080 8080
 $base='http://127.0.0.1:18080';$tag=[Guid]::NewGuid().ToString('N')
 function Public([string]$Path,[string]$Method='GET',$Body=$null,[string]$Key=''){
  $args=@{Uri="$base$Path";Method=$Method;TimeoutSec=8;SkipHttpErrorCheck=$true}
  if($Method -ne 'GET'){$args.Headers=@{'Idempotency-Key'=$Key};$args.ContentType='application/json';$args.Body=$Body|ConvertTo-Json -Depth 5 -Compress}
  Invoke-WebRequest @args
 }
 $r=Public /requests POST @{type='SHELTER';location=@{latitude=-40;longitude=150};details='Day4 public contract fixture'} "request-$tag"
 if([int]$r.StatusCode -ne 201){throw "Create request failed $($r.StatusCode)"};$request=$r.Content|ConvertFrom-Json
 Kube-Wait {((Public "/requests/$($request.id)").Content|ConvertFrom-Json).status -eq 'UNMATCHED'} 'empty search exhausted'
 $wrong=Public /resources POST @{type='FOOD';location=@{latitude=-40;longitude=150};availability=$true} "wrong-$tag"
 $far=Public /resources POST @{type='SHELTER';location=@{latitude=-39;longitude=150};availability=$true} "far-$tag"
 $eligible=Public /resources POST @{type='SHELTER';location=@{latitude=-39.95;longitude=150};availability=$false} "eligible-$tag"
 foreach($response in @($wrong,$far,$eligible)){if([int]$response.StatusCode -ne 201){throw 'Resource create failed'}}
 $resource=$eligible.Content|ConvertFrom-Json
 $r=Public "/resources/$($resource.id)/status" PUT @{status='AVAILABLE'} "available-$tag"
 if([int]$r.StatusCode -ne 200){throw 'Resource status failed'}
 $r=Public "/match/$($request.id)" POST @{} "match-$tag"
 if([int]$r.StatusCode -ne 200){throw 'Manual schedule failed'}
 Kube-Wait {((Public "/requests/$($request.id)").Content|ConvertFrom-Json).status -eq 'ASSIGNED'} 'eligible resource assigned'
 $assigned=(Public "/requests/$($request.id)").Content|ConvertFrom-Json
 if($assigned.assignedResourceId -ne $resource.id){throw 'Wrong type or outside-radius resource assigned'}
 $after='';$found=$false
 do{$r=Public "/resources?limit=100&after=$after";if([int]$r.StatusCode -ne 200){throw 'Resource list failed'};$page=$r.Content|ConvertFrom-Json;if($resource.id -in $page.items.id){$found=$true};$after=$page.next}while($after -and !$found)
 if(!$found){throw 'Resource missing from global listing'}
 $r=Public "/requests/$($request.id)/status" PUT @{status='COMPLETED'} "complete-$tag"
 if([int]$r.StatusCode -ne 200 -or ($r.Content|ConvertFrom-Json).status -ne 'COMPLETED'){throw 'Completion failed'}
 $r=Public "/resources/$($resource.id)/status" PUT @{status='AVAILABLE'} "release-$tag"
 if([int]$r.StatusCode -ne 409){throw 'Allocated resource released'}
 $r=Public /requests POST @{type='WATER';location=@{latitude=91;longitude=0};details='invalid'} "invalid-$tag"
 if([int]$r.StatusCode -ne 400){throw 'Invalid coordinates accepted'}
 if((Public /requests/00000000000000000000000000000000).StatusCode -ne 404){throw 'Missing request not404'}
 if(!(Invoke-WebRequest "$base/").Content.Contains('Request status')){throw 'Browser page missing'}
 Write-Output "PASS: all seven public APIs, 5-to-10km expansion, wrong-type/far exclusion, unmatched/manual retry, completion/resource protection, invalid400/missing404. Request=$($request.id) Resource=$($resource.id)"
}finally{if($forward -and !$forward.HasExited){Stop-Process -Id $forward.Id}}
