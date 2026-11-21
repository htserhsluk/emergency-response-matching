'use strict';
// Display only: all eligibility and allocation decisions belong to the server.
let responseMap,selectionLayer,assignmentLayer,currentMappedRequest='',mappedSignature='',selectionRevision=0;
const mapElement=id=>document.getElementById(id);
let recordsLayer, detailLayer, records=new Map(), inspected=null;
let locationsVersion=0, detailVersion=0, mapPoll, locationsSignature='', detailSignature='', detailFit='';
const pointOf=location=>[location.latitude,location.longitude];
function pin(point,kind,label,options={}){
 const symbols={selection:'◆',request:'R',resource:'+',candidate:'?',assigned:'✓'};
 const icon=L.divIcon({className:'location-pin '+kind,html:'<span>'+(options.rank||symbols[kind])+'</span>',iconSize:[32,40],iconAnchor:[16,40],tooltipAnchor:[0,-32]});
 const marker=L.marker(point,{icon,title:label,alt:label,interactive:!!options.select,keyboard:!!options.select,zIndexOffset:options.priority||0});
 if(options.select)marker.on('click',options.select);
 return marker.bindTooltip(label);
}
function mapOption(value,label){
 const option=document.createElement('option');
 option.value=value;option.textContent=label;
 return option;
}
const recordKey=r=>r.kind+'/'+r.id;
const typeName=type=>type?type.charAt(0)+type.slice(1).toLowerCase():'Saved';
const recordLabel=r=>typeName(r.type)+' '+r.kind;
const mapIncludes=type=>mapElement('map-type').value==='ALL'||mapElement('map-type').value===type;
let popupVersion=0;
function popupContent(record,notice=''){
 const box=document.createElement('div');
 box.className='record-popup';
 const title=document.createElement('strong');title.textContent=recordLabel(record);box.append(title);
 const fields=[
  ['ID',record.id],['Assistance type',record.type],['Status',record.status],
  ['Coordinates',pointOf(record.location).join(', ')],['Details',record.details],
  ['Available',record.availability],['Created',record.createdAt],['Updated',record.updatedAt],
  ['Registered',record.registeredAt],['Allocated',record.allocatedAt],['Allocation confirmed',record.allocationConfirmedAt],['Generation',record.generation],
  ['Assigned resource',record.assignedResourceId||record.assignment?.resourceId],['Assigned request',record.assignedRequestId],
  ['Distance (m)',record.distanceMeters??record.assignment?.distanceMeters],
  ['Matching',record.matching?.state],['Search radius (m)',record.matching?.radiusMeters],
  ['Assignment coordinates',record.assignment?.location&&pointOf(record.assignment.location).join(', ')]
 ];
 for(const [label,value] of fields){
  if(value===undefined||value===null||value==='')continue;
  const line=document.createElement('div');line.textContent=label+': '+value;box.append(line);
 }
 if(notice){const line=document.createElement('div');line.textContent=notice;box.append(line);}
 return box;
}
async function showRecordPopup(r){
 const version=++popupVersion;
 const popup=L.popup({autoPan:false,maxWidth:280,maxHeight:230,offset:[0,-36]})
  .setLatLng(pointOf(r.location)).setContent(popupContent(r)).openOn(responseMap);
 try{
  const full=await api('/'+r.kind+'s/'+encodeURIComponent(r.id));
  if(version===popupVersion)popup.setContent(popupContent({...r,...full}));
 }catch(e){if(version===popupVersion)popup.setContent(popupContent(r,'Additional details temporarily unavailable.'));}
}
function savedPin(record,style=record.kind,options={}){
 return pin(pointOf(record.location),style,recordLabel(record),{...options,select:()=>{
  if(options.inspect)inspectRecord(record);
  showRecordPopup(record);
 }});
}
function drawOverview(fit=false){
 recordsLayer.clearLayers();
 for(const r of records.values())savedPin(r,r.kind,{inspect:true,priority:100}).addTo(recordsLayer);
 if(fit&&records.size)responseMap.fitBounds([...records.values()].map(r=>pointOf(r.location)),{padding:[40,45],maxZoom:14,animate:false});
}
function clearInspection(){
 ++detailVersion;++popupVersion;++selectionRevision;
 inspected=null;detailSignature='';detailFit='';
 responseMap.closePopup();detailLayer.clearLayers();clearMappedRequest();selectionLayer.clearLayers();
 mapElement('map-record').value='';
 mapElement('map-detail').textContent='';
}
function focusMapRequest(id){
 if(!responseMap||!detailLayer)return;
 if(inspected?.kind==='request'&&inspected.id===id)return;
 clearInspection();recordsLayer.clearLayers();
 inspected=records.get('request/'+id)||{kind:'request',id};
 mapElement('map-record').value=recordKey(inspected);
}
function inspectRecord(record){
 clearInspection();recordsLayer.clearLayers();
 if(!record){drawOverview(true);return;}
 inspected=record;
 mapElement('map-record').value=recordKey(record);
 if(record.kind==='request'){
  mapElement('map-detail').textContent=recordLabel(record)+' '+record.id+' · '+record.status;
  savedPin(record).addTo(assignmentLayer);
  follow(record.id);
 }else{
  mapElement('map-detail').textContent='Loading nearby requests…';
  savedPin(record).addTo(detailLayer);
  refreshCandidates();
 }
}
async function refreshCandidates(){
 if(inspected?.kind!=='resource')return;
 const resource=inspected,version=++detailVersion;
 try{
  const data=await api('/resources/'+encodeURIComponent(resource.id)+'/candidates');
  if(version!==detailVersion)return;
  const assigned=data.assignedRequest,candidates=assigned?[]:data.items.slice(0,5);
  const origin=pointOf(resource.location),points=[origin];
  const signature=JSON.stringify([resource,candidates,assigned]);
  if(signature===detailSignature)return;
  detailLayer.clearLayers();
  savedPin(resource,'resource',{priority:400}).addTo(detailLayer);
  const lines=['Resource '+resource.id+' · '+resource.status];
  candidates.forEach((request,index)=>{
   const point=pointOf(request.location);points.push(point);
   savedPin({kind:'request',type:resource.type,...records.get('request/'+request.id),...request},'candidate',{rank:index+1,priority:500}).addTo(detailLayer);
   lines.push((index+1)+'. Eligible request '+request.id+' · '+Math.round(request.distanceMeters).toLocaleString()+' m');
  });
  if(!candidates.length&&!assigned)lines.push('No eligible requests.');
  if(assigned){
   const point=pointOf(assigned.location);points.push(point);
   L.polyline([origin,point],{color:'#16665b',weight:4,interactive:false}).addTo(detailLayer);
   savedPin({kind:'request',type:resource.type,...records.get('request/'+assigned.id),...assigned},'assigned',{priority:600}).addTo(detailLayer);
   lines.push('Assigned request '+assigned.id+' · '+Math.round(assigned.distanceMeters).toLocaleString()+' m');
  }
  mapElement('map-detail').textContent=lines.join('\n');
  const fit=resource.id;
  if(fit!==detailFit){responseMap.fitBounds(points,{padding:[40,45],maxZoom:14,animate:false});detailFit=fit;}
  detailSignature=signature;
 }catch(e){
  if(version===detailVersion){detailSignature='';mapElement('map-detail').textContent='Could not refresh nearby requests. Retrying automatically.';}
 }
}
async function refreshLocations(fitOverview=false){
 if(!recordsLayer)return;
 clearTimeout(mapPoll);
 const version=++locationsVersion,type=mapElement('map-type').value,collected=new Map();
 try{
  let after='';
  do{
   const query=new URLSearchParams({type,after});
   const page=await api('/map/locations?'+query);
   if(version!==locationsVersion)return;
   for(const record of page.items)collected.set(recordKey(record),record);
   after=page.next||'';
  }while(after);
  const signature=JSON.stringify([...collected]);
  if(signature!==locationsSignature){
   records=collected;
   if(typeof createdResource!=='undefined'&&createdResource){
    const saved=records.get('resource/'+createdResource.id);
    if(saved?.status==='ALLOCATED'&&createdResource.status!=='ALLOCATED'){
     showCreatedResource({...createdResource,...saved,availability:false});
    }
   }
   const picker=mapElement('map-record');
   picker.replaceChildren(mapOption('','All locations'));
   for(const [key,r] of records){
    const label=recordLabel(r)+' · '+r.status+' · '+r.id;
    picker.append(mapOption(key,label));

   }
   if(inspected&&records.has(recordKey(inspected))){inspected=records.get(recordKey(inspected));picker.value=recordKey(inspected);}
   if(!inspected)drawOverview(fitOverview);
   locationsSignature=signature;
  }
  mapElement('locations-note').textContent=records.size+' saved locations · '+(type==='ALL'?'All types':typeName(type))+'.';
  await refreshCandidates();
 }catch(e){if(version===locationsVersion)mapElement('locations-note').textContent='Could not refresh saved locations. Retrying automatically.';}
 finally{if(version===locationsVersion)mapPoll=setTimeout(refreshLocations,5000);}
}
function setupRecords(){
 recordsLayer=L.layerGroup().addTo(responseMap);detailLayer=L.layerGroup().addTo(responseMap);
 mapElement('map-record').addEventListener('change',e=>inspectRecord(records.get(e.target.value)));
 mapElement('map-type').addEventListener('change',()=>{
  clearInspection();recordsLayer.clearLayers();records.clear();locationsSignature='';
  mapElement('map-record').replaceChildren(mapOption('','All locations'));
  mapElement('locations-note').textContent='Loading saved locations…';refreshLocations(true);
 });
 refreshLocations(true);
}
function drawAssignment(r){
 if(!responseMap||inspected?.kind!=='request'||inspected.id!==r.id)return;
 if(r.type&&!mapIncludes(r.type)){inspectRecord(null);return;}
 inspected={...r,kind:'request'};
 mapElement('map-detail').textContent=recordLabel(inspected)+' '+r.id+' · '+r.status;
 const signature=JSON.stringify([r.id,r.location,r.assignment&&r.assignment.resourceId,r.assignment&&r.assignment.location]);
 if(signature===mappedSignature)return;
 const changedRequest=currentMappedRequest!==r.id;
 assignmentLayer.clearLayers();
 const requestPoint=[r.location.latitude,r.location.longitude],points=[requestPoint];
 savedPin(inspected).addTo(assignmentLayer);
 const a=r.assignment;
 if(a&&a.location){
  const p=[a.location.latitude,a.location.longitude];points.push(p);
  savedPin({kind:'resource',id:a.resourceId,type:r.type,assignedRequestId:r.id,...records.get('resource/'+a.resourceId),location:a.location}).addTo(assignmentLayer);
  L.polyline([requestPoint,p],{color:'#16665b',dashArray:'5 5'}).addTo(assignmentLayer);
 }
 // Fit once on selection so background status changes preserve the viewport.
 if(changedRequest){responseMap.fitBounds(points,{padding:[45,45],maxZoom:14,animate:false});}
 currentMappedRequest=r.id;mappedSignature=signature;
}
function clearMappedRequest(){if(assignmentLayer)assignmentLayer.clearLayers();currentMappedRequest='';mappedSignature='';}
function selectLocation(lat,lon,pan=false,writeFields=true){
 selectionRevision++;
 if(writeFields){mapElement('latitude').value=Number(lat).toFixed(6);mapElement('longitude').value=Number(lon).toFixed(6);}
 if(!responseMap)return;
 if(inspected){clearInspection();drawOverview();}
 selectionLayer.clearLayers();pin([lat,lon],'selection','Selected location').addTo(selectionLayer);
 if(pan)responseMap.setView([lat,lon],12,{animate:false});
}
function setupLocation(){
 mapElement('use-location').onclick=()=>{
  const feedback=mapElement('location-feedback'),button=mapElement('use-location');
  if(!navigator.geolocation){feedback.textContent='Device location is unsupported. Enter coordinates or select the map.';return;}
  const revision=++selectionRevision;button.disabled=true;feedback.textContent='Locating… your browser may ask for permission.';
  navigator.geolocation.getCurrentPosition(position=>{
   button.disabled=false;if(revision!==selectionRevision){feedback.textContent='Location result ignored because you changed the selection.';return;}
   const {latitude,longitude,accuracy}=position.coords;
   if(!Number.isFinite(latitude)||!Number.isFinite(longitude)||Math.abs(latitude)>90||Math.abs(longitude)>180||!Number.isFinite(accuracy)||accuracy<0){feedback.textContent='Device returned an invalid location. Please select manually.';return;}
   if(accuracy>1000){feedback.textContent=`Location is too approximate (±${Math.round(accuracy)} m). Your selection was kept; enter coordinates or select the map.`;return;}
   selectLocation(latitude,longitude,true);feedback.textContent=`Location selected (approximately ±${Math.round(accuracy)} m). Check and adjust it before submitting.`;
  },error=>{button.disabled=false;if(revision!==selectionRevision)return;feedback.textContent=({1:'Location permission denied.',2:'Device location unavailable.',3:'Location request timed out.'}[error.code]||'Could not get device location.')+' Enter coordinates or select the map instead.';},{enableHighAccuracy:true,timeout:10000,maximumAge:0});
 };
 for(const id of ['latitude','longitude']){
  mapElement(id).addEventListener('input',()=>{
   selectionRevision++;
   const latitude=mapElement('latitude').value,longitude=mapElement('longitude').value;
   const lat=Number(latitude),lon=Number(longitude);
   // Keep partial typing intact; an empty number field is not latitude zero.
   if(latitude.trim()&&longitude.trim()&&Number.isFinite(lat)&&Number.isFinite(lon)&&Math.abs(lat)<=90&&Math.abs(lon)<=180)selectLocation(lat,lon,false,false);
  });
  mapElement(id).addEventListener('change',()=>{try{const p=locationInput();selectLocation(p.latitude,p.longitude,true);}catch(e){message(e.message,true);}});
 }
}
async function initializeMap(){
 setupLocation();
 if(!window.L){mapElement('map-note').textContent='Map library unavailable. Manual coordinates and device location still work.';return;}
 responseMap=L.map('map',{scrollWheelZoom:false}).setView([12.9,77.6],12);
 selectionLayer=L.layerGroup().addTo(responseMap);assignmentLayer=L.layerGroup().addTo(responseMap);
 setupRecords();
 responseMap.on('click',e=>selectLocation(Math.max(-90,Math.min(90,e.latlng.lat)),((e.latlng.lng+180)%360+360)%360-180));
 if(typeof ResizeObserver!=='undefined')new ResizeObserver(()=>responseMap.invalidateSize({pan:false})).observe(mapElement('map'));
 try{
  const config=await api('/web-config');
  if(!config.tileUrl||new URLSearchParams(location.search).get('tiles')==='off'){mapElement('map-note').textContent='Background tiles disabled. Location pins and manual coordinates remain available.';return;}
  const text=document.createElement('span');text.textContent=config.attribution;
  const layer=L.tileLayer(config.tileUrl,{maxZoom:19,attribution:text.innerHTML+' · <a href="https://www.openstreetmap.org/fixthemap" target="_blank" rel="noopener">Report a map issue</a>'});
  let slowTimer,tileFailed=false;
  const tileFailure=()=>{mapElement('map-note').textContent='Some background tiles unavailable. Pins and coordinates still work; matching is unaffected.';};
  // Attach before adding the layer so the initial loading event is not missed.
  layer.on('loading',()=>{
   clearTimeout(slowTimer);tileFailed=false;
   mapElement('map-note').textContent='Loading background map… pins and coordinates are ready to use.';
   slowTimer=setTimeout(()=>{if(!tileFailed)mapElement('map-note').textContent='Background map is taking longer to load from the tile provider. You can keep using pins and coordinates; matching is unaffected.';},8000);
  });
  layer.on('tileerror',()=>{tileFailed=true;clearTimeout(slowTimer);tileFailure();});
  layer.on('load',()=>{clearTimeout(slowTimer);if(tileFailed)tileFailure();else mapElement('map-note').textContent='Background map loaded. Select the map or enter coordinates; the legend identifies each pin.';});
  layer.addTo(responseMap);
 }catch(e){mapElement('map-note').textContent='Map configuration unavailable. Pins and coordinates remain available.';}
}
initializeMap();
