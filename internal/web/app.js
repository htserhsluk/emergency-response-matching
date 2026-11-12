'use strict';
const $=id=>document.getElementById(id);
let selected='', timer, nextResource='', lastRequest, nextNotification='', followGeneration=0;
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
 lastRequest=r;
 const box=$('request-result');box.replaceChildren(el('span',r.status,'badge'),el('p',`Request ${r.id}`),el('p',`${r.type} • ${r.location.latitude}, ${r.location.longitude}`),el('p',r.details));
 if(r.assignedResourceId)box.append(el('p',`Assigned resource: ${r.assignedResourceId}`));
 if(r.assignment){box.append(el('p',`Observed distance: ${r.assignment.distanceMeters.toFixed(1)} metres`));if(r.assignment.location){box.append(el('p',`Resource location: ${r.assignment.location.latitude}, ${r.assignment.location.longitude}`));}else{box.append(el('p','Resource location temporarily unavailable; assignment status is saved.'));}}
 if(typeof drawAssignment==='function')drawAssignment(r);
 $('complete').disabled=r.status!=='ASSIGNED';$('match').disabled=!['PENDING','UNMATCHED'].includes(r.status);
}
async function refresh(generation=followGeneration){if(!selected)return;const id=selected;const r=await api('/requests/'+id);if(selected===id&&generation===followGeneration){displayRequest(r);await notifications(id,'',generation);}return r;}
function follow(id){const generation=++followGeneration;selected=id;lastRequest=null;$('request-id').value=id;$('request-result').textContent='Loading saved status…';$('inbox-list').textContent='Loading inbox…';$('match').disabled=true;$('complete').disabled=true;clearTimeout(timer);const poll=async()=>{let delay=3000;try{await refresh(generation);}catch(e){if(generation===followGeneration)message('Status or inbox refresh failed: '+e.message,true);delay=10000;}if(generation===followGeneration)timer=setTimeout(poll,delay);};poll();}
$('request').onclick=()=>action($('request'),async()=>{if(!$('details').value.trim())throw new Error('Describe the request before submitting.');const r=await api('/requests','POST',{type:$('type').value,location:locationInput(),details:$('details').value});follow(r.id);message('Request saved. Matching is pending; watch its status below.');});
$('resource').onclick=()=>action($('resource'),async()=>{const r=await api('/resources','POST',{type:$('type').value,location:locationInput(),availability:true});message('Available resource registered: '+r.id);await resources();});
$('lookup').onsubmit=e=>{e.preventDefault();follow($('request-id').value.trim());};
$('match').onclick=()=>action($('match'),async()=>{if(!selected)throw new Error('Look up a request first.');await api('/match/'+selected,'POST',{});message('Matching scheduled. The saved status will update asynchronously.');await refresh();});
$('complete').onclick=()=>action($('complete'),async()=>{if(!selected)throw new Error('Look up a request first.');await api('/requests/'+selected+'/status','PUT',{status:'COMPLETED'});message('Request marked completed.');await refresh();});
async function resources(after=''){
 const page=await api('/resources?limit=20&after='+encodeURIComponent(after)),box=$('resource-list');box.replaceChildren();
 for(const r of page.items){const row=el('div','','record');row.append(el('strong',`${r.type} / ${r.status}`),el('p',r.id),el('p',`${r.location.latitude}, ${r.location.longitude}`));
 if(r.status!=='ALLOCATED'){const button=el('button',r.availability?'Mark unavailable':'Mark available','secondary');button.onclick=()=>action(button,async()=>{await api('/resources/'+r.id+'/status','PUT',{status:r.availability?'UNAVAILABLE':'AVAILABLE'});await resources(after);});row.append(button);}box.append(row);}
 if(!page.items.length)box.append(el('p','No resources on this page.'));nextResource=page.next||'';$('more-resources').hidden=!nextResource;
}
$('resources').onclick=()=>action($('resources'),()=>resources());$('more-resources').onclick=()=>action($('more-resources'),()=>resources(nextResource));
$('match').disabled=true;$('complete').disabled=true;

async function notifications(id,after='',generation=followGeneration){
 const page=await api('/requests/'+id+'/notifications?limit=20&after='+encodeURIComponent(after));if(selected!==id||generation!==followGeneration)return;
 const box=$('inbox-list');box.replaceChildren();
 for(const n of page.items){const row=el('div','','record');row.append(el('strong',n.readAt?'Assignment notification · Read':'Assignment notification · Unread'),el('p','Resource '+n.resourceId),el('p','Inbox created '+new Date(n.createdAt).toLocaleString()));
  if(!n.readAt){const button=el('button','Mark as read','secondary');button.onclick=()=>action(button,async()=>{await api('/requests/'+id+'/notifications/'+n.id+'/read','PUT',{});await notifications(id);});row.append(button);}box.append(row);
 }
 if(!page.items.length)box.append(el('p','No assignment notifications for this request.'));
 nextNotification=page.nextAfter||'';$('more-notifications').hidden=!nextNotification;
}
$('more-notifications').onclick=()=>action($('more-notifications'),()=>notifications(selected,nextNotification));
