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
function follow(id){const generation=++followGeneration;selected=id;lastRequest=null;if(typeof focusMapRequest==='function')focusMapRequest(id);$('request-id').value=id;$('request-result').textContent='Loading saved status…';$('inbox-list').textContent='Loading inbox…';$('match').disabled=true;$('complete').disabled=true;clearTimeout(timer);const poll=async()=>{let delay=3000;try{await refresh(generation);}catch(e){if(generation===followGeneration)message('Status or inbox refresh failed: '+e.message,true);delay=10000;}if(generation===followGeneration)timer=setTimeout(poll,delay);};poll();}
$('request').onclick=()=>action($('request'),async()=>{if(!$('details').value.trim())throw new Error('Describe the request before submitting.');const r=await api('/requests','POST',{type:$('type').value,location:locationInput(),details:$('details').value});if(typeof clearInspection==='function'&&detailLayer){clearInspection();refreshLocations();}follow(r.id);message('Request saved. Matching starts automatically. Follow the saved status below.');});
$('resource').onclick=()=>action($('resource'),async()=>{const r=await api('/resources','POST',{type:$('type').value,location:locationInput(),availability:true});message('Available resource registered: '+r.id);showCreatedResource(r);if(typeof inspectRecord==='function'&&recordsLayer&&mapIncludes(r.type)){inspectRecord({...r,kind:'resource'});refreshLocations();}await resources();});
$('lookup').onsubmit=e=>{e.preventDefault();follow($('request-id').value.trim());};
$('match').onclick=()=>action($('match'),async()=>{if(!selected)throw new Error('Look up a request first.');await api('/match/'+selected,'POST',{});message('Matching scheduled. The saved status will update asynchronously.');await refresh();});
$('complete').onclick=()=>action($('complete'),async()=>{if(!selected)throw new Error('Look up a request first.');await api('/requests/'+selected+'/status','PUT',{status:'COMPLETED'});message('Request marked completed.');await refresh();});
function resourceName(r){
 return r.type.charAt(0)+r.type.slice(1).toLowerCase()+' resource';
}
function resourceRow(r){
 const button=el('button','','resource-entry');
 button.type='button';
 button.id='resource-entry-'+r.id;
 button.setAttribute('aria-haspopup','dialog');
 button.append(el('strong',resourceName(r)),el('span',r.id));
 button.onclick=()=>openRecordDetails({...r,kind:'resource'},button);
 return button;
}
let recordView=null, detailRead=0;
function availabilityControl(r){
 const button=el('button',r.availability?'Mark unavailable':'Mark available','secondary');
 button.onclick=()=>action(button,async()=>{
  let updated;
  try{
   updated=await api('/resources/'+r.id+'/status','PUT',{status:r.availability?'UNAVAILABLE':'AVAILABLE'});
  }catch(error){
   if(recordView?.resource?.id===r.id)$('resource-dialog-feedback').textContent=error.message;
   throw error;
  }
  if(createdResource?.id===updated.id)showCreatedResource(updated);
  if(recordView?.resource?.id===updated.id){
   ++detailRead;
   recordView.resource=updated;
   recordView.pending=null;
   recordView.errors={};
   renderRecordDetails(recordView);
   $('record-detail-retry').hidden=true;
   $('resource-dialog-feedback').textContent='Resource availability updated.';
   ($('resource-dialog-actions').firstElementChild||$('resource-dialog-close')).focus();
  }
  message('Resource availability updated.');
  await resources();
 });
 return button;
}
function detailSection(kind,title,rows){
 const section=el('section','');
 section.setAttribute('data-detail',kind);
 const fields=el('dl','');
 for(const [label,value] of rows)fields.append(el('dt',label),el('dd',String(value)));
 section.append(el('h3',title),fields);
 return section;
}
function renderRecordDetails(view){
 const {resource,request,pair,kind}=view;
 const fields=$('resource-dialog-fields'),actions=$('resource-dialog-actions');
 function missing(entity){
  if(view.pending===entity)return 'Loading…';
  return view.errors[entity]?'Temporarily unavailable':'Not recorded';
 }
 function time(value,fallback){return value?new Date(value).toLocaleString():fallback;}
 const sections=[];
 if(kind==='resource'||pair){
  const absent=missing('resource');
  sections.push(detailSection('resource','Resource',[
   ['Resource ID',resource?.id||request?.assignedResourceId||absent],['Assistance type',resource?.type??absent],
   ['Status',resource?.status??absent],['Available for matching',resource?.availability==null?absent:(resource.availability?'Yes':'No')],
   ['Latitude',resource?.location?.latitude??request?.assignment?.location?.latitude??absent],
   ['Longitude',resource?.location?.longitude??request?.assignment?.location?.longitude??absent],['Registered',time(resource?.registeredAt,absent)]
  ]));
 }
 if(kind==='request'||pair){
  const absent=missing('request');
  const rows=[['Request ID',request?.id||resource?.assignedRequestId||absent],['Description',request?.details??absent],['Assistance type',request?.type??absent],
   ['Status',request?.status??absent],['Latitude',request?.location?.latitude??absent],['Longitude',request?.location?.longitude??absent],['Created',time(request?.createdAt,absent)]];
  if(request?.matching){
   rows.push(['Matching state',request.matching.state],['Search radius',request.matching.radiusMeters+' metres']);
   if(request.matching.reason)rows.push(['Matching reason',request.matching.reason]);
  }
  if(request?.generation!=null)rows.push(['Generation',request.generation]);
  sections.push(detailSection('request',pair?'Matched request':'Request',rows));
 }
 if(pair||resource?.assignedRequestId){
  const distance=pair?request?.assignment?.distanceMeters:null;
  const rows=pair?
   [['Assigned resource ID',resource?.id||request.assignedResourceId],['Matched request ID',request?.id||resource.assignedRequestId],
    ['Saved distance',distance==null?missing('request'):distance+' metres']]:
   [['Reserved resource ID',resource.id],['Linked request ID',resource.assignedRequestId]];
  if(resource?.allocatedAt)rows.push(['Allocated',time(resource.allocatedAt)]);
  else if(resource?.allocationConfirmedAt)rows.push(['Allocation confirmed',time(resource.allocationConfirmedAt)]);
  else if(resource?.allocationConfirmationUnavailable)rows.push(['Allocation confirmed','Temporarily unavailable']);
  else rows.push(['Allocation time',missing('resource')]);
  sections.push(detailSection('assignment',pair?'Assignment':'Reservation',rows));
 }
 fields.replaceChildren(...sections);
 const primary=view[kind];
 const typeName=primary?.type?primary.type.charAt(0)+primary.type.slice(1).toLowerCase()+' ':'';
 $('resource-dialog-title').textContent=pair?'Assignment details':typeName+kind;
 actions.replaceChildren();
 if(kind==='resource'&&resource?.status&&resource.status!=='ALLOCATED'&&typeof resource.availability==='boolean'){
  const button=availabilityControl(resource);
  button.disabled=!!view.pending||!!view.errors.resource;
  actions.append(button);
 }
}
async function loadRecordDetails(view){
 const sequence=++detailRead;
 const feedback=$('resource-dialog-feedback'),retry=$('record-detail-retry');
 const other=view.kind==='resource'?'request':'resource';
 retry.hidden=true;
 feedback.textContent='Checking current details.';
 async function refresh(kind,id){
  view.pending=kind;
  renderRecordDetails(view);
  try{
   const current=await api('/'+kind+'s/'+encodeURIComponent(id));
   if(sequence!==detailRead)return;
   delete view.errors[kind];
   const previous=view[kind];
   if(current.allocationConfirmationUnavailable && previous?.assignedRequestId===current.assignedRequestId && previous?.allocationConfirmedAt){
    current.allocationConfirmedAt=previous.allocationConfirmedAt;
   }
   view[kind]=current;
  }catch(error){
   if(sequence===detailRead)view.errors[kind]=error.message;
  }
 }
 await refresh(view.kind,view.id);
 if(sequence!==detailRead)return;
 const primary=view[view.kind];
 const linked=view.kind==='resource'?primary.assignedRequestId:
  (['ASSIGNED','COMPLETED'].includes(primary.status)&&primary.assignedResourceId);
 if(view[other]?.id!==linked){
  if(!linked||view[other])delete view.errors[other];
  view[other]=null;
  view.pair=false;
 }
 if(linked){
  // A finalized request names the resource even when that shard is offline.
  if(view.kind==='request')view.pair=true;
  await refresh(other,linked);
  if(sequence!==detailRead)return;
  const {resource,request}=view;
  if(resource&&request)view.pair=resource.assignedRequestId===request.id&&request.assignedResourceId===resource.id&&
   ['ASSIGNED','COMPLETED'].includes(request.status)&&(!request.assignment||request.assignment.resourceId===resource.id);
 }
 view.pending=null;
 const messages=Object.entries(view.errors).map(([entity,error])=>'Could not load current '+entity+' details. '+error);
 if(view.resource?.allocationConfirmationUnavailable)messages.push('Allocation confirmation time is temporarily unavailable.');
 if(linked&&!view.pair&&!view.errors[other])messages.push('The linked records do not currently confirm this assignment.');
 feedback.textContent=messages.join(' ');
 retry.hidden=messages.length===0;
 renderRecordDetails(view);
}
function openRecordDetails(record,opener,restoreFocus){
 const view={kind:record.kind,id:record.id,opener,restoreFocus,errors:{},[record.kind]:record};
 recordView=view;
 const dialog=$('resource-dialog');
 if(!dialog.open)dialog.showModal();
 return loadRecordDetails(view);
}
$('record-detail-retry').onclick=()=>{
 if(!recordView)return;
 $('resource-dialog-close').focus();
 return loadRecordDetails(recordView);
};
$('resource-dialog-close').onclick=()=>$('resource-dialog').close();
$('resource-dialog').onclose=()=>{
 if($('resource-dialog').open)return;
 ++detailRead;
 const view=recordView;
 recordView=null;
 if(!view)return;
 if(view.restoreFocus){view.restoreFocus();return;}
 const target=view.opener?.isConnected?view.opener:$(view.opener?.id)||$('resource-filter');
 target.focus({preventScroll:true});
};
function showCreatedResource(r){
 ++createdRefresh;
 createdResource=r;
 const summary=el('div','','record');
 summary.append(el('strong',resourceName(r)+' / '+r.status),el('p',r.id),el('p',r.location.latitude+', '+r.location.longitude));
 if(r.assignedRequestId)summary.append(el('p','Assigned request: '+r.assignedRequestId));
 if(r.status!=='ALLOCATED')summary.append(availabilityControl(r));
 $('created-resource').replaceChildren(el('h3','Recently registered resource'),summary);
}
async function refreshCreatedResource(){if(!createdResource)return;const id=createdResource.id,sequence=++createdRefresh;try{const r=await api('/resources/'+id);if(sequence===createdRefresh&&createdResource.id===id)showCreatedResource(r);}catch(e){if(sequence===createdRefresh)$('created-resource').replaceChildren(el('h3','Recently registered resource'),el('p',id),el('p','Current status unavailable. Refresh to retry.'));}}
async function resources(after=''){
 const generation=++resourceGeneration,filter=resourceFilter,feedback=$('resource-feedback');
 refreshCreatedResource();feedback.textContent='Loading resources…';$('more-resources').disabled=true;
 try{
  const order=filter==='ALLOCATED'?'allocated':'newest';
  const page=await api('/resources?sort='+order+'&limit=20&after='+encodeURIComponent(after)+'&status='+encodeURIComponent(filter));
  if(generation!==resourceGeneration)return;
  const fingerprint=JSON.stringify({filter,after,page}),unchanged=fingerprint===resourceFingerprint;
  const box=$('resource-list');
  if(!unchanged){
   const focused=document.activeElement;
   const restore=focused&&box.contains(focused);
   box.replaceChildren();
   for(const r of page.items)box.append(resourceRow(r));
   if(!page.items.length)box.append(el('p','No resources in this view. Try another filter or register assistance.'));
   if(restore)($(focused.id)||$('resource-filter')).focus();
  }
  nextResource=page.next||'';$('more-resources').hidden=!nextResource;
  resourceFingerprint=fingerprint;resourceLoaded=true;
  feedback.textContent=unchanged?'Refresh complete — results unchanged.':`Loaded ${page.items.length} resources. ${filter==='ALLOCATED'?'Most recent allocations first.':'Newest registrations first.'}`;
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
