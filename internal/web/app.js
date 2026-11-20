'use strict';
const $=id=>document.getElementById(id);
let selected='', timer, nextResource='', lastRequest, nextNotification='', followGeneration=0;
let requestRefresh=0, notificationRefresh=0, createdResource=null, createdRefresh=0;
let resourceGeneration=0, resourceFilter='UNALLOCATED', resourceFingerprint='', resourceLoaded=false;
const pending=new Map();
function message(text,error=false){$('message').textContent=text;$('message').classList.toggle('error',error);}
async function api(path,method='GET',body){
 const payload=body===undefined?undefined:JSON.stringify(body), signature=method+path+(payload||'');
 let key=pending.get(signature);
 if(method!=='GET'&&!key){key=crypto.randomUUID();pending.set(signature,key);}
 let response;
 try{response=await fetch(path,{method,headers:{'Content-Type':'application/json',...(key?{'Idempotency-Key':key}:{})},body:payload,signal:AbortSignal.timeout(8000)});}catch(e){throw new Error('Connection unavailable. The outcome may be pending; retry the same action to reuse its operation key.');}
 const data=await response.json();
 if(!response.ok){if(response.status<500)pending.delete(signature);throw new Error(`${data.message||'Request failed'} (${response.status})`);}
 pending.delete(signature);return data;
}
function locationInput(){
 const lat=$('latitude'),lon=$('longitude');
 if(!lat.reportValidity()||!lon.reportValidity()||lat.value===''||lon.value==='')throw new Error('Enter valid latitude and longitude.');
 return {latitude:Number(lat.value),longitude:Number(lon.value)};
}
function el(tag,text,className){const e=document.createElement(tag);e.textContent=text;if(className)e.className=className;return e;}
async function action(button,run){button.disabled=true;try{await run();}catch(e){message(e.message,true);}finally{button.disabled=false;if(lastRequest){$('complete').disabled=lastRequest.status!=='ASSIGNED';$('match').disabled=!['PENDING','UNMATCHED'].includes(lastRequest.status);}}}
function displayRequest(r){
 const previousAssignment=lastRequest&&lastRequest.assignedResourceId;
 lastRequest=r;
 if(resourceLoaded&&r.assignedResourceId&&previousAssignment!==r.assignedResourceId)resources();
 const box=$('request-result');box.replaceChildren(el('span',r.status,'badge'),el('p',`Request ${r.id}`),el('p',`${r.type} • ${r.location.latitude}, ${r.location.longitude}`),el('p',r.details));
 if(r.matching){
  const labels={SEARCHING:'Search in progress',RETRYING:'Waiting to confirm processing; retry is scheduled',LEGACY_UNMATCHED:'No resource found under the previous 10 km policy; automatic matching can search up to 4,000 km',PAUSED:'Processing paused after repeated failures; operator attention is needed',EXHAUSTED:'No eligible resource found within 4,000 km. Matching resumes when assistance becomes available',ASSIGNED:'Allocation saved'};
  box.append(el('p',(labels[r.matching.state]||r.matching.state)+` · Current search radius: ${(r.matching.radiusMeters/1000).toLocaleString()} km`));
 }
 if(r.assignedResourceId)box.append(el('p',`Assigned resource: ${r.assignedResourceId}`));
 if(r.assignment){box.append(el('p',`Observed distance: ${r.assignment.distanceMeters.toFixed(1)} metres`));if(r.assignment.location){box.append(el('p',`Resource location: ${r.assignment.location.latitude}, ${r.assignment.location.longitude}`));}else{box.append(el('p','Resource location temporarily unavailable; assignment status is saved.'));}}
 if(typeof drawAssignment==='function')drawAssignment(r);
 $('complete').disabled=r.status!=='ASSIGNED';$('match').disabled=!['PENDING','UNMATCHED'].includes(r.status);
}
async function refresh(generation=followGeneration){if(!selected)return;const id=selected,sequence=++requestRefresh;const r=await api('/requests/'+id);if(selected===id&&generation===followGeneration&&sequence===requestRefresh){displayRequest(r);await notifications(id,'',generation);}return r;}
function follow(id){const generation=++followGeneration;selected=id;lastRequest=null;if(typeof clearMappedRequest==='function')clearMappedRequest();$('request-id').value=id;$('request-result').textContent='Loading saved status…';$('inbox-list').textContent='Loading inbox…';$('match').disabled=true;$('complete').disabled=true;clearTimeout(timer);const poll=async()=>{let delay=3000;try{await refresh(generation);}catch(e){if(generation===followGeneration)message('Status or inbox refresh failed: '+e.message,true);delay=10000;}if(generation===followGeneration)timer=setTimeout(poll,delay);};poll();}
$('request').onclick=()=>action($('request'),async()=>{if(!$('details').value.trim())throw new Error('Describe the request before submitting.');const r=await api('/requests','POST',{type:$('type').value,location:locationInput(),details:$('details').value});if(typeof clearInspection==='function'&&detailLayer){clearInspection();refreshLocations();}follow(r.id);message('Request saved. Matching starts automatically. Follow the saved status below.');});
$('resource').onclick=()=>action($('resource'),async()=>{const r=await api('/resources','POST',{type:$('type').value,location:locationInput(),availability:true});message('Available resource registered: '+r.id);showCreatedResource(r);if(typeof inspectRecord==='function'&&recordsLayer&&r.type===type.value){inspectRecord({...r,kind:'resource'});refreshLocations();}await resources();});
$('lookup').onsubmit=e=>{e.preventDefault();follow($('request-id').value.trim());};
$('match').onclick=()=>action($('match'),async()=>{if(!selected)throw new Error('Look up a request first.');await api('/match/'+selected,'POST',{});message('Matching scheduled. The saved status will update asynchronously.');await refresh();});
$('complete').onclick=()=>action($('complete'),async()=>{if(!selected)throw new Error('Look up a request first.');await api('/requests/'+selected+'/status','PUT',{status:'COMPLETED'});message('Request marked completed.');await refresh();});
function resourceRow(r){
 const row=el('div','','record');row.append(el('strong',`${r.type} / ${r.status}`),el('p',r.id),el('p',`${r.location.latitude}, ${r.location.longitude}`));
 if(r.assignedRequestId)row.append(el('p','Assigned request: '+r.assignedRequestId));
 if(r.status!=='ALLOCATED'){
  const button=el('button',r.availability?'Mark unavailable':'Mark available','secondary');
  button.onclick=()=>action(button,async()=>{const updated=await api('/resources/'+r.id+'/status','PUT',{status:r.availability?'UNAVAILABLE':'AVAILABLE'});if(createdResource&&createdResource.id===updated.id)showCreatedResource(updated);message('Resource availability updated.');await resources();});row.append(button);
 }
 return row;
}
function showCreatedResource(r){++createdRefresh;createdResource=r;$('created-resource').replaceChildren(el('h3','Recently registered resource'),resourceRow(r));}
async function refreshCreatedResource(){if(!createdResource)return;const id=createdResource.id,sequence=++createdRefresh;try{const r=await api('/resources/'+id);if(sequence===createdRefresh&&createdResource.id===id)showCreatedResource(r);}catch(e){if(sequence===createdRefresh)$('created-resource').replaceChildren(el('h3','Recently registered resource'),el('p',id),el('p','Current status unavailable. Refresh to retry.'));}}
async function resources(after=''){
 const generation=++resourceGeneration,filter=resourceFilter,feedback=$('resource-feedback');
 refreshCreatedResource();feedback.textContent='Loading resources…';$('more-resources').disabled=true;
 try{
  const page=await api('/resources?sort=newest&limit=20&after='+encodeURIComponent(after)+'&status='+encodeURIComponent(filter));
  if(generation!==resourceGeneration)return;
  const fingerprint=JSON.stringify({filter,after,page}),unchanged=fingerprint===resourceFingerprint;
  const box=$('resource-list');box.replaceChildren();
  for(const r of page.items)box.append(resourceRow(r));
  if(!page.items.length)box.append(el('p','No resources in this view. Try another filter or register assistance.'));
  nextResource=page.next||'';$('more-resources').hidden=!nextResource;
  resourceFingerprint=fingerprint;resourceLoaded=true;
  feedback.textContent=unchanged?'Refresh complete — results unchanged.':`Loaded ${page.items.length} resources. Newest registrations first.`;
 }catch(e){if(generation===resourceGeneration){feedback.textContent='Refresh failed. Previous results may be out of date. '+e.message;} }
 finally{if(generation===resourceGeneration)$('more-resources').disabled=false;}
}
$('resources').onclick=()=>action($('resources'),()=>resources());
$('more-resources').onclick=()=>action($('more-resources'),()=>resources(nextResource));
$('resource-filter').onchange=()=>{resourceFilter=$('resource-filter').value;nextResource='';$('more-resources').hidden=true;resources();};
$('match').disabled=true;$('complete').disabled=true;

async function notifications(id,after='',generation=followGeneration){
 const sequence=++notificationRefresh;const page=await api('/requests/'+id+'/notifications?limit=20&after='+encodeURIComponent(after));if(selected!==id||generation!==followGeneration||sequence!==notificationRefresh)return;
 const box=$('inbox-list');box.replaceChildren();
 for(const n of page.items){const row=el('div','','record');row.append(el('strong',n.readAt?'Assignment notification · Read':'Assignment notification · Unread'),el('p','Resource '+n.resourceId),el('p','Inbox created '+new Date(n.createdAt).toLocaleString()));
  if(!n.readAt){const button=el('button','Mark as read','secondary');button.onclick=()=>action(button,async()=>{await api('/requests/'+id+'/notifications/'+n.id+'/read','PUT',{});await notifications(id);});row.append(button);}box.append(row);
 }
 if(!page.items.length)box.append(el('p','No assignment notifications for this request.'));
 nextNotification=page.nextAfter||'';$('more-notifications').hidden=!nextNotification;
}
$('more-notifications').onclick=()=>action($('more-notifications'),()=>notifications(selected,nextNotification));
