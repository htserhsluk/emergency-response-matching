'use strict';
// Maps only display server results. They never choose an assignment.
let responseMap,selectionLayer,assignmentLayer,currentMappedRequest='';
function drawAssignment(r){
 if(!responseMap)return;
 assignmentLayer.clearLayers();
 const requestPoint=[r.location.latitude,r.location.longitude];
 L.circleMarker(requestPoint,{radius:8,color:'#2d6686'}).bindTooltip('Request '+r.id).addTo(assignmentLayer);
 const a=r.assignment;
 if(a&&a.location){const p=[a.location.latitude,a.location.longitude];L.circleMarker(p,{radius:7,color:'#17694f'}).bindTooltip('Assigned resource '+a.resourceId).addTo(assignmentLayer);L.polyline([requestPoint,p],{color:'#17694f',dashArray:'5 5'}).addTo(assignmentLayer);}
 if(currentMappedRequest!==r.id){responseMap.setView(requestPoint,12);currentMappedRequest=r.id;}
}
async function initializeMap(){
 if(!window.L){document.getElementById('map-note').textContent='Map library unavailable. Coordinate fields and request tracking still work.';return;}
 responseMap=L.map('map',{scrollWheelZoom:false}).setView([12.9,77.6],12);
 selectionLayer=L.layerGroup().addTo(responseMap);assignmentLayer=L.layerGroup().addTo(responseMap);
 function select(lat,lon){selectionLayer.clearLayers();L.circleMarker([lat,lon],{radius:7,color:'#ac6b17'}).bindTooltip('Selected location').addTo(selectionLayer);}
 responseMap.on('click',e=>{const lon=((e.latlng.lng+180)%360+360)%360-180;document.getElementById('latitude').value=e.latlng.lat.toFixed(6);document.getElementById('longitude').value=lon.toFixed(6);select(e.latlng.lat,lon);});
 for(const id of ['latitude','longitude'])document.getElementById(id).addEventListener('change',()=>{try{const p=locationInput();select(p.latitude,p.longitude);responseMap.setView([p.latitude,p.longitude],12);}catch(e){message(e.message,true);}});
 try{
  const config=await api('/web-config');
  if(!config.tileUrl || new URLSearchParams(location.search).get('tiles')==='off'){document.getElementById('map-note').textContent='Background tiles disabled. Markers and coordinate fields remain available.';return;}
  const text=document.createElement('span');text.textContent=config.attribution;
  const layer=L.tileLayer(config.tileUrl,{maxZoom:19,attribution:text.innerHTML+' · <a href="https://www.openstreetmap.org/fixthemap" target="_blank" rel="noopener">Report a map issue</a>'}).addTo(responseMap);
  layer.on('tileerror',()=>{document.getElementById('map-note').textContent='Background tiles unavailable. Use the coordinate fields; saved status and matching are unaffected.';});
  document.getElementById('map-note').textContent='Click to choose coordinates. Blue: request · Green: assigned resource · Amber: selection.';
 }catch(e){document.getElementById('map-note').textContent='Map configuration unavailable. Coordinate fields and request tracking still work.';}
}
initializeMap();
