'use strict';
// Display only: all eligibility and allocation decisions belong to the server.
let responseMap,selectionLayer,assignmentLayer,currentMappedRequest='',mappedSignature='',lastFitSignature='',selectionRevision=0;
const mapElement=id=>document.getElementById(id);
function pin(point,kind,label){
 const symbols={selection:'◆',request:'R',resource:'+'};
 const icon=L.divIcon({className:'location-pin '+kind,html:'<span>'+symbols[kind]+'</span>',iconSize:[32,40],iconAnchor:[16,40]});
 return L.marker(point,{icon,title:label,alt:label,zIndexOffset:kind==='selection'?300:kind==='request'?200:100}).bindTooltip(label);
}
function drawAssignment(r){
 if(!responseMap)return;
 const signature=JSON.stringify([r.id,r.location,r.assignment&&r.assignment.resourceId,r.assignment&&r.assignment.location]);
 if(signature===mappedSignature)return;
 const changedRequest=currentMappedRequest!==r.id;
 assignmentLayer.clearLayers();
 const requestPoint=[r.location.latitude,r.location.longitude],points=[requestPoint];
 pin(requestPoint,'request','Request '+r.id).addTo(assignmentLayer);
 const a=r.assignment;
 if(a&&a.location){
  const p=[a.location.latitude,a.location.longitude];points.push(p);
  pin(p,'resource','Assigned resource '+a.resourceId).addTo(assignmentLayer);
  L.polyline([requestPoint,p],{color:'#16665b',dashArray:'5 5'}).addTo(assignmentLayer);
 }
 // Only new identities/coordinates alter the view; polling does not reset a pan.
 if(changedRequest||(points.length>1&&signature!==lastFitSignature)){responseMap.fitBounds(points,{padding:[45,45],maxZoom:14});lastFitSignature=signature;}
 currentMappedRequest=r.id;mappedSignature=signature;
}
function clearMappedRequest(){if(assignmentLayer)assignmentLayer.clearLayers();currentMappedRequest='';mappedSignature='';lastFitSignature='';}
function selectLocation(lat,lon,pan=false,writeFields=true){
 selectionRevision++;
 if(writeFields){mapElement('latitude').value=Number(lat).toFixed(6);mapElement('longitude').value=Number(lon).toFixed(6);}
 if(!responseMap)return;
 selectionLayer.clearLayers();pin([lat,lon],'selection','Selected location').addTo(selectionLayer);
 if(pan)responseMap.setView([lat,lon],12);
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
 const initial=locationInput();selectLocation(initial.latitude,initial.longitude);
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
