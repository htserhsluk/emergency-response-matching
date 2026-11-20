const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const os=require('node:os');
const path=require('node:path');
const http=require('node:http');
const {spawn}=require('node:child_process');

const browser=[process.env.CHROME_BIN,
  'C:/Program Files/Google/Chrome/Application/chrome.exe',
  '/usr/bin/chromium','/usr/bin/google-chrome'
].find(file=>file&&fs.existsSync(file));

test('Leaflet marker tips stay on saved coordinates through zoom and resize',{skip:!browser},async()=>{
 const directory=fs.mkdtempSync(path.join(os.tmpdir(),'erm-map-'));
 const nativeAssets=new Set(['/styles.css','/map.js','/app.js','/vendor/leaflet/leaflet.js','/vendor/leaflet/leaflet.css']);
 const location={latitude:12.91,longitude:77.61};
 const resource={id:'fixture',kind:'resource',type:'WATER',status:'ALLOCATED',location};
 const responses={
  '/map/locations':{items:[resource]},
  '/resources/fixture/candidates':{items:[{id:'near',location:{latitude:12.912,longitude:77.611},distanceMeters:250}],assignedRequest:{id:'saved',location,distanceMeters:0}}
 };
 const probe=`<script>
 const responses=${JSON.stringify(responses)};
 const sendResult=window.fetch.bind(window);
 window.fetch=async path=>({ok:true,json:async()=>responses[path.split('?')[0]]||{}});
 window.addEventListener('load',async()=>{
  try{
   const picker=document.getElementById('map-record');
   picker.value='resource/fixture';picker.dispatchEvent(new Event('change'));
   await new Promise(resolve=>setTimeout(resolve,100));
   const mapNode=document.getElementById('map');let checks=0;
   for(const [zoom,width] of [[8,440],[15,280],[12,520]]){
    mapNode.style.width=width+'px';
    responseMap.invalidateSize();responseMap.setView([12.912,77.61],zoom,{animate:false});
    for(const group of [selectionLayer,recordsLayer,detailLayer])for(const marker of group.getLayers()){
     if(!(marker instanceof L.Marker))continue;
     const pixel=responseMap.latLngToContainerPoint(marker.getLatLng());
     const node=marker.getElement(),rect=node.getBoundingClientRect(),origin=mapNode.getBoundingClientRect();
     const tip=getComputedStyle(node,'::after');
     const x=rect.left+parseFloat(tip.left)+parseFloat(tip.borderLeftWidth)-origin.left;
     const y=rect.bottom-parseFloat(tip.bottom)-origin.top;
     if(Math.max(Math.abs(x-pixel.x),Math.abs(y-pixel.y))>1)throw Error(node.className+' zoom '+zoom+' measured '+x+','+y+' expected '+pixel.x+','+pixel.y+' geometry '+rect.toJSON().width+'x'+rect.toJSON().height+' anchor '+tip.left+'/'+tip.bottom);
     checks++;
    }
   }
   if(!document.getElementById('map-detail').textContent.includes('Assigned request saved'))throw Error('missing assignment');
   await sendResult('/result',{method:'POST',body:'VERIFIED '+checks+' marker positions'});
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
  assert.equal(await Promise.race([result,timeout]),'VERIFIED 15 marker positions');
 }finally{
  clearTimeout(timer);if(child)child.kill();server.closeAllConnections();server.close();
  try{fs.rmSync(directory,{recursive:true,force:true,maxRetries:3});}catch(error){if(!['EBUSY','ENOTEMPTY','EPERM'].includes(error.code))throw error;}
 }
});
