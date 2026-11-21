const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const os=require('node:os');
const path=require('node:path');
const http=require('node:http');
const {spawn}=require('node:child_process');

const {chrome:browser}=require('./browser_fixture.cjs');

test('Leaflet marker tips stay on saved coordinates through zoom and resize',{skip:!browser},async()=>{
 const directory=fs.mkdtempSync(path.join(os.tmpdir(),'erm-map-'));
 const nativeAssets=new Set(['/styles.css','/map.js','/app.js','/vendor/leaflet/leaflet.js','/vendor/leaflet/leaflet.css']);
 const location={latitude:12.91,longitude:77.61};
 const resource={id:'water-resource-000123',kind:'resource',type:'WATER',status:'AVAILABLE',location};
 const request={id:'water-request-000456',kind:'request',type:'WATER',status:'PENDING',location:{latitude:12.912,longitude:77.611}};
 const medicine={...resource,id:'medicine-resource-000789',type:'MEDICINE'};
 const records=[resource,request,medicine];
 const probe=`<script>
 const fixture=${JSON.stringify(records)},[water,need,medicine]=fixture;
 const sendResult=window.fetch.bind(window);
 let assigned=false,overviewChanged=false;
 const candidate={id:need.id,location:need.location,distanceMeters:250};
 window.fetch=async path=>{
  const url=new URL(path,location.href),route=url.pathname;
  let body={items:[]};
  if(route==='/map/locations'){
   const type=url.searchParams.get('type');
   body={items:fixture.filter(r=>r.type===type||type==='ALL').map(r=>r.id===water.id&&overviewChanged?{...r,status:'UNAVAILABLE'}:r)};
  }else if(route.endsWith('/candidates')){
   body=assigned?{items:[],assignedRequest:candidate}:{items:[candidate]};
  }else if(route==='/requests/'+need.id){
   body={...need,details:'Drinking water for the clinic',generation:3,createdAt:'2026-11-20T12:00:00Z'};
   if(assigned)Object.assign(body,{status:'ASSIGNED',assignedResourceId:water.id,assignment:{resourceId:water.id,location:water.location,distanceMeters:250}});
  }else if(route==='/resources/'+water.id){body={...water,status:assigned?'ALLOCATED':'AVAILABLE',assignedRequestId:assigned?need.id:undefined,availability:!assigned,registeredAt:'2026-11-20T12:00:00Z',allocatedAt:'2026-11-20T13:00:00Z',allocationConfirmedAt:'2026-11-20T14:00:00Z'};}
  return {ok:true,json:async()=>body};
 };
 window.addEventListener('load',async()=>{
  try{
   const picker=document.getElementById('map-record'),filter=document.getElementById('map-type'),creation=document.getElementById('type');
   const mapNode=document.getElementById('map');
   const settle=()=>new Promise(resolve=>setTimeout(resolve,30));
   const check=(condition,message)=>{if(!condition)throw Error(message);};
   const select=async(element,value)=>{element.value=value;element.dispatchEvent(new Event('change'));await settle();};
   const markers=()=>[...mapNode.querySelectorAll('.location-pin')];
   const checkPins=(labels)=>{
    const actual=markers().map(node=>node.title).sort();
    check(JSON.stringify(actual)===JSON.stringify(labels.sort()),'Unexpected pins: '+JSON.stringify(actual));
   };
   const open=async(label,key)=>{
    const node=markers().find(node=>node.title===label);check(node,'Missing '+label);
    node.dispatchEvent(new MouseEvent('mouseover',{bubbles:true}));
    const tips=[...mapNode.querySelectorAll('.leaflet-tooltip')].map(tip=>tip.textContent);
    check(tips.includes(label),'Hover '+label+' produced '+JSON.stringify(tips));
    if(key){node.focus({preventScroll:true});node.dispatchEvent(new KeyboardEvent('keydown',{key,bubbles:true,cancelable:true}));}
    else node.dispatchEvent(new MouseEvent('click',{bubbles:true}));
    await settle();
    check(!mapNode.querySelector('.leaflet-popup'),'Unexpected Leaflet popup');
    const dialog=document.querySelector('dialog[open]');check(dialog,'Missing details dialog');
    return dialog.textContent;
   };
   const close=async(label)=>{
    const dialog=document.querySelector('dialog[open]');dialog.querySelector('button[autofocus]').click();await settle();
    check(!dialog.open,'Dialog still open');check(document.activeElement.title===label,'Wrong focus after dismissing '+label);
   };
   await settle();
   check(filter.value==='ALL'&&picker.selectedOptions[0].textContent==='All locations','Wrong default filters');
   checkPins(['Water resource','Water request','Medicine resource']);
   await select(filter,'WATER');checkPins(['Water resource','Water request']);
   await select(creation,'SHELTER');check(filter.value==='WATER','Map filter changed with creation form');
   responseMap.fire('click',{latlng:L.latLng(12.91,77.61)});
   check(markers().some(node=>node.classList.contains('selection')),'Draft point missing');
   const overviewZoom=responseMap.getZoom(),overviewCenter=responseMap.getCenter();
   let content=await open('Water resource','Enter');
   for(const text of [water.id,'WATER','AVAILABLE','12.91','77.61'])check(content.includes(text),'Missing resource detail '+text);
   const oldMarker=markers().find(node=>node.title==='Water resource');
   overviewChanged=true;await refreshLocations();await settle();
   check(!oldMarker.isConnected,'Overview marker was not replaced');
   await close('Water resource');
   check(picker.value===''&&filter.value==='WATER','Inspecting pin changed overview selection');
   check(responseMap.getZoom()===overviewZoom&&responseMap.getCenter().equals(overviewCenter),'Inspecting pin moved overview');
   await select(picker,'resource/'+water.id);checkPins(['Water resource','Water request']);
   content=await open('Water request',' ');
   for(const text of [need.id,'WATER','PENDING','12.912','77.611','Drinking water for the clinic'])check(content.includes(text),'Missing candidate detail '+text);
   await close('Water request');
   let checks=0;
   for(const [zoom,width] of [[8,440],[15,280],[12,520]]){
    mapNode.style.width=width+'px';
    responseMap.invalidateSize();responseMap.setView([12.912,77.61],zoom,{animate:false});
    for(const marker of detailLayer.getLayers()){
     if(!(marker instanceof L.Marker))continue;
     const pixel=responseMap.latLngToContainerPoint(marker.getLatLng());
     const node=marker.getElement(),rect=node.getBoundingClientRect(),origin=mapNode.getBoundingClientRect();
     const tip=getComputedStyle(node,'::after');
     const x=rect.left+parseFloat(tip.left)+parseFloat(tip.borderLeftWidth)-origin.left;
     const y=rect.bottom-parseFloat(tip.bottom)-origin.top;
     check(Math.max(Math.abs(x-pixel.x),Math.abs(y-pixel.y))<=1,'Marker tip moved at zoom '+zoom);
     checks++;
    }
   }
   await settle();const center=responseMap.getCenter();
   await open('Water request');assigned=true;await refreshLocations();await settle();
   await close('Water request');
   check(document.activeElement.classList.contains('assigned'),'Focus did not follow candidate to confirmed marker');
   check(responseMap.getZoom()===12&&responseMap.getCenter().equals(center),'Refresh changed zoom/center: '+center+' to '+responseMap.getCenter()+' at '+responseMap.getZoom());
   check(picker.value==='resource/'+water.id,'Refresh changed selection');
   checkPins(['Water resource','Water request']);
   check(mapNode.querySelector('.location-pin.assigned')&&!mapNode.querySelector('.location-pin.candidate'),'Missing confirmed assignment');
   content=await open('Water request');check(content.includes(water.id)&&content.includes('ASSIGNED'),'Confirmed dialog missing assignment');await close('Water request');
   await select(picker,'request/'+need.id);checkPins(['Water request','Water resource']);
   content=await open('Water resource');check(content.includes(water.id),'Assigned resource dialog missing');await close('Water resource');
   await select(picker,'');checkPins(['Water request','Water resource']);
   await refresh();checkPins(['Water request','Water resource']);
   await select(filter,'ALL');checkPins(['Water resource','Water request','Medicine resource']);
   check(creation.value==='SHELTER','Creation type changed with map filter');
   await sendResult('/result',{method:'POST',body:'VERIFIED '+checks+' marker positions and map inspections'});
  }catch(error){await sendResult('/result',{method:'POST',body:'FAILED '+error.message});}
 });
 </script>`;
 const html=fs.readFileSync(path.join(__dirname,'index.html'),'utf8').replace('</head>',probe+'</head>');
 let completed;
 const result=new Promise(resolve=>{completed=resolve;});
 const server=http.createServer((req,res)=>{
  if(req.url==='/result'){
   let body='';req.setEncoding('utf8');req.on('data',part=>body+=part);
   req.on('end',()=>{res.end();completed(body);});
  }else if(req.url==='/'){res.setHeader('Content-Type','text/html');res.end(html);}
  else if(nativeAssets.has(req.url)){
   res.setHeader('Content-Type',req.url.endsWith('.css')?'text/css':'application/javascript');
   res.end(fs.readFileSync(path.join(__dirname,req.url.slice(1))));
  }else{res.writeHead(404);res.end();}
 });
 let child,timer;
 try{
  await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
  child=spawn(browser,['--headless','--disable-extensions','--disable-component-update','--disable-sync','--disable-background-networking','--disable-gpu','--no-first-run','--user-data-dir='+path.join(directory,'profile'),'http://127.0.0.1:'+server.address().port],{stdio:'ignore',windowsHide:true});
  const timeout=new Promise((_,reject)=>{timer=setTimeout(()=>reject(Error('Browser did not return marker measurements')),60000);child.on('error',reject);});
  assert.equal(await Promise.race([result,timeout]),'VERIFIED 6 marker positions and map inspections');
 }finally{
  clearTimeout(timer);if(child)child.kill();server.closeAllConnections();server.close();
  try{fs.rmSync(directory,{recursive:true,force:true,maxRetries:3});}catch(error){if(!['EBUSY','ENOTEMPTY','EPERM'].includes(error.code))throw error;}
 }
});
