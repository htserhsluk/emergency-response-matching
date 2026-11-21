const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
function harness(geo,tiles=false,apiOverride){
 const elements=new Map(),groups=[],views=[],timers=new Map(),dialogs=[];let resize,timerId=0;const tileEvents={};
 const element=()=>({value:'',textContent:'',disabled:false,children:[],focus(){this.focused=true;},setAttribute(){},append(...nodes){this.children.push(...nodes);},replaceChildren(...nodes){this.children=nodes;},addEventListener(name,fn){this[name]=fn;}});
 const get=id=>{if(!elements.has(id))elements.set(id,element());return elements.get(id);};
 get('map-type').value='ALL';get('type').value='WATER';get('latitude').value='12.9';get('longitude').value='77.6';
 const map={setView(p){views.push(p);return this;},fitBounds(p){views.push(p);return this;},on(name,fn){this[name]=fn;return this;},invalidateSize(){}};
 const marker=(point,options)=>({point,options,node:element(),getElement(){return this.node;},on(event,fn){this[event]=fn;return this;},bindTooltip(label){this.tooltip=label;return this;},addTo(g){g.items.push(this);this.add?.();return this;}});
 const L={tileLayer(){return {addTo(){if(tileEvents.loading)tileEvents.loading();return this;},on(event,fn){tileEvents[event]=fn;return this;}};},map:()=>map,layerGroup(){const g={items:[],getLayers(){return this.items;},addTo(){groups.push(this);return this;},clearLayers(){this.items=[];}};return g;},marker,circleMarker:marker,polyline:marker,divIcon:o=>o,latLngBounds:p=>p};
 const context=vm.createContext({openRecordDetails(record,opener,restoreFocus){dialogs.push({record,opener,restoreFocus});},window:{L},L,document:{getElementById:get,createElement:element},navigator:geo?{geolocation:geo}:{},location:{search:tiles?'':'?tiles=off'},URLSearchParams,ResizeObserver:class{constructor(fn){resize=fn;}observe(){}},api:apiOverride||(async path=>path.startsWith('/map/locations')?{items:[]}:tiles?({tileUrl:"https://example.invalid/{z}/{x}/{y}.png",attribution:"Fixture tiles"}):({})),locationInput:()=>({latitude:Number(get('latitude').value),longitude:Number(get('longitude').value)}),message(){},setTimeout:fn=>fn()});
 context.setTimeout=fn=>{timers.set(++timerId,fn);return timerId;};context.clearTimeout=id=>timers.delete(id);
 vm.runInContext(fs.readFileSync(__dirname+'/map.js','utf8'),context);
 return {context,get,groups,views,map,tileEvents,dialogs,resize:()=>resize(),tick(){const pending=[...timers.values()];timers.clear();pending.forEach(fn=>fn());}};
}
test('map starts in the saved-locations overview without a draft marker',()=>{
 const h=harness();assert.equal(h.groups[0].items.length,0);
});
test('coordinate input moves the selection without rewriting typing or jumping the view',()=>{
 const h=harness(),views=h.views.length;
 h.get('latitude').value='-65.00';h.get('longitude').value='43.0';h.get('longitude').input();
 assert.deepEqual(Array.from(h.groups[0].items[0].point),[-65,43]);
 assert.equal(h.get('latitude').value,'-65.00');assert.equal(h.get('longitude').value,'43.0');
 assert.equal(h.groups[0].items.length,1);assert.equal(h.views.length,views);
 for(const value of ['', '-', '91']){
  h.get('latitude').value=value;h.get('latitude').input();
  assert.deepEqual(Array.from(h.groups[0].items[0].point),[-65,43]);
 }
 h.map.click({latlng:{lat:15,lng:76}});
 assert.deepEqual(Array.from(h.groups[0].items[0].point),[15,76]);
 assert.equal(h.get('latitude').value,'15.000000');assert.equal(h.get('longitude').value,'76.000000');
});
test('map fits assignment once and does not jump on identical polling',()=>{
 const h=harness();const r={id:'r',location:{latitude:0,longitude:0},assignment:{resourceId:'s',location:{latitude:15,longitude:76}}};
 h.context.fixture=r;vm.runInContext('focusMapRequest(fixture.id);drawAssignment(fixture)',h.context);assert.equal(h.views.at(-1).length,2);assert.ok(Array.isArray(h.views.at(-1)[0]),'fit both points');const n=h.views.length;
 vm.runInContext('drawAssignment(fixture)',h.context);assert.equal(h.views.length,n);assert.equal(h.groups[1].items.length,3);
});
test('geolocation denial leaves manual coordinates intact',()=>{
 const h=harness({getCurrentPosition(ok,fail){fail({code:1});}});
 h.get('use-location').onclick();assert.match(h.get('location-feedback').textContent,/denied/i);assert.equal(Number(h.get('latitude').value),12.9);
});

test('geolocation success, low accuracy, timeout and unsupported outcomes',()=>{
 const success=harness({getCurrentPosition(ok){ok({coords:{latitude:0,longitude:-76,accuracy:20}});}});
 success.get('use-location').onclick();assert.equal(Number(success.get('latitude').value),0);assert.equal(Number(success.get('longitude').value),-76);assert.match(success.get('location-feedback').textContent,/approximately/);
 const poor=harness({getCurrentPosition(ok){ok({coords:{latitude:40,longitude:20,accuracy:5000}});}});
 poor.get('use-location').onclick();assert.equal(Number(poor.get('latitude').value),12.9);assert.match(poor.get('location-feedback').textContent,/too approximate/);
 const timeout=harness({getCurrentPosition(ok,fail){fail({code:3});}});timeout.get('use-location').onclick();assert.match(timeout.get('location-feedback').textContent,/timed out/);
 const unsupported=harness();unsupported.get('use-location').onclick();assert.match(unsupported.get('location-feedback').textContent,/unsupported/);
});

test('late geolocation cannot overwrite a manual edit and request switching clears old markers',()=>{
 let finish;const h=harness({getCurrentPosition(ok){finish=ok;}});
 h.get('use-location').onclick();h.get('latitude').value='20';h.get('latitude').input();
 finish({coords:{latitude:50,longitude:60,accuracy:10}});assert.equal(h.get('latitude').value,'20');
 h.context.fixture={id:'one',location:{latitude:0,longitude:0},assignment:{resourceId:'two',location:{latitude:0,longitude:0}}};
 vm.runInContext('focusMapRequest(fixture.id);drawAssignment(fixture);clearMappedRequest()',h.context);assert.equal(h.groups[1].items.length,0);
});

test('temporary missing resource coordinates do not reset the users map view when polling recovers',()=>{
 const h=harness();h.context.fixture={id:'one',location:{latitude:0,longitude:0},assignment:{resourceId:'two',location:{latitude:1,longitude:1}}};
 vm.runInContext('focusMapRequest(fixture.id);drawAssignment(fixture)',h.context);const n=h.views.length;
 vm.runInContext('drawAssignment({...fixture,assignment:{resourceId:"two"}});drawAssignment(fixture)',h.context);
 assert.equal(h.views.length,n);
});

test('tile failure and resize preserve usable selection',async()=>{
 const h=harness(undefined,true);await new Promise(setImmediate);
 h.map.click({latlng:{lat:12,lng:77}});h.tileEvents.tileerror();h.resize();
 assert.match(h.get('map-note').textContent,/tiles unavailable/);assert.equal(h.groups[0].items.length,1);
});
test('tile loading reports slow responses, completion and recoverable failure',async()=>{
 const h=harness(undefined,true);await new Promise(setImmediate);
 h.map.click({latlng:{lat:12,lng:77}});assert.match(h.get('map-note').textContent,/loading/i);
 h.tick();assert.match(h.get('map-note').textContent,/taking longer/i);
 h.tileEvents.load();assert.match(h.get('map-note').textContent,/loaded/i);
 h.tileEvents.loading();h.tileEvents.tileerror();h.tileEvents.load();
 assert.match(h.get('map-note').textContent,/unavailable/i);
 h.tileEvents.loading();h.tileEvents.load();h.tick();
 assert.match(h.get('map-note').textContent,/loaded/i);assert.equal(h.groups[0].items.length,1);
});

test('all pin symbols have a geographic tip at the declared anchor',()=>{
 const h=harness();
 for(const kind of ['selection','resource','request']){
  const icon=vm.runInContext(`pin([12,77],'${kind}','Example').options.icon`,h.context);
  assert.deepEqual(Array.from(icon.iconAnchor),[icon.iconSize[0]/2,icon.iconSize[1]]);
 }
 const css=fs.readFileSync(__dirname+'/styles.css','utf8');
 assert.doesNotMatch(css,/\.location-pin[^{}]*\{[^{}]*transform:\s*translate/);
 assert.match(css,/\.location-pin[^{}]*::after/);
});
test('map pages all matching locations and selecting a resource draws candidates and a single confirmed link',async()=>{
 const location={latitude:12,longitude:77},resource={id:'s',kind:'resource',type:'WATER',status:'ALLOCATED',location};
 const request={id:'r',kind:'request',type:'WATER',status:'ASSIGNED',location},paths=[];
 const h=harness(undefined,false,async path=>{
  paths.push(path);
  if(path.startsWith('/map/locations'))return path.includes('after=next')?{items:[request]}:{items:[resource],next:'next'};
  if(path.includes('/candidates'))return {items:[{id:'eligible',location:{latitude:13,longitude:78},distanceMeters:100}],assignedRequest:{id:'r',location,distanceMeters:0}};
  return {};
 });
 await new Promise(setImmediate);
 assert.ok(paths.some(path=>path.includes('type=ALL')&&path.includes('after=next')));
 const recordPins=h.groups.flatMap(group=>group.items).filter(marker=>marker.click);
 const resourcePin=recordPins.find(marker=>marker.options.title.includes('resource')||marker.options.title.includes('Resource'));
 assert.ok(resourcePin);resourcePin.click();await new Promise(setImmediate);
 assert.equal(h.dialogs[0]?.record.id,'s');
 assert.equal(h.groups[2].items.length,2);
 assert.equal(h.get('map-record').value,'');
 h.get('map-record').change({target:{value:'resource/s'}});await new Promise(setImmediate);
 assert.ok(paths.includes('/resources/s/candidates'));
 assert.equal(h.groups.flatMap(group=>group.items).filter(marker=>Array.isArray(marker.point[0])).length,1);
 assert.doesNotMatch(h.get('map-detail').textContent,/eligible/i);
 assert.match(h.get('map-detail').textContent,/assigned.*r/i);
});
test('changing map type while an earlier page is loading discards stale records',async()=>{
 let finish;
 const h=harness(undefined,false,async path=>{
  if(path.includes('type=ALL'))return new Promise(resolve=>finish=resolve);
  return {items:[]};
 });
 await new Promise(setImmediate);h.get('map-type').value='FOOD';h.get('map-type').change();await new Promise(setImmediate);
 finish({items:[{id:'stale',kind:'resource',location:{latitude:0,longitude:0}}]});await new Promise(setImmediate);
 assert.equal(h.groups[2].items.length,0);
 assert.equal(h.get('map-record').children.some(option=>option.value==='resource/stale'),false);
});
test('request polling cannot draw a second assignment while inspecting a resource or another type',()=>{
 const h=harness();
 h.get('map-type').value='WATER';vm.runInContext("focusMapRequest('old')",h.context);h.context.fixture={id:'old',type:'FOOD',location:{latitude:0,longitude:0},assignment:{resourceId:'used',location:{latitude:1,longitude:1}}};
 vm.runInContext('drawAssignment(fixture)',h.context);
 assert.equal(h.groups[1].items.length,0);
 vm.runInContext("inspected={kind:'resource'};drawAssignment({...fixture,type:'WATER'})",h.context);
 assert.equal(h.groups[1].items.length,0);
});
test('resource polling replaces the preview with its confirmed assignment and ignores late detail responses',async()=>{
 const location={latitude:0,longitude:0},s={id:'one',kind:'resource',type:'WATER',status:'AVAILABLE',location};
 const other={...s,id:'two'},pending=[];
 const h=harness(undefined,false,async path=>{
  if(path.includes('/map/locations'))return {items:[s,other]};
  if(path.endsWith('/candidates'))return new Promise(resolve=>pending.push(resolve));
  return {};
 });
 await new Promise(setImmediate);
 const select=id=>h.get('map-record').change({target:{value:'resource/'+id}});
 select('one');select('two');
 pending[1]({items:[{id:'eligible-request',location,distanceMeters:0}]});await new Promise(setImmediate);
 pending[0]({items:[],assignedRequest:{id:'old-resource-match',location,distanceMeters:0}});await new Promise(setImmediate);
 assert.match(h.get('map-detail').textContent,/eligible-request/);
 assert.doesNotMatch(h.get('map-detail').textContent,/old-resource-match/);
 h.tick();await new Promise(setImmediate);
 pending[2]({items:[],assignedRequest:{id:'confirmed-request',location,distanceMeters:0}});await new Promise(setImmediate);
 assert.match(h.get('map-detail').textContent,/Assigned request confirmed-request/);
 assert.doesNotMatch(h.get('map-detail').textContent,/eligible-request/);
 assert.equal(h.groups.flatMap(group=>group.items).filter(marker=>Array.isArray(marker.point[0])).length,1);
});
test('a saved-location refresh updates the recently registered resource card after automatic allocation',async()=>{
 const location={latitude:0,longitude:0},s={id:'one',kind:'resource',type:'WATER',status:'AVAILABLE',location};
 const h=harness(undefined,false,async path=>path.includes('/map/locations')?{items:[{...s,status:'ALLOCATED',assignedRequestId:'saved'}]}:{});
 h.context.createdResource=s;
 let saved;
 h.context.showCreatedResource=updated=>{saved=updated;};
 await new Promise(setImmediate);
 assert.equal(saved?.status,'ALLOCATED');assert.equal(saved?.assignedRequestId,'saved');
});
test('inspecting a saved resource hides draft and other saved pins, and clicks disclose record details',async()=>{
 const location={latitude:12,longitude:77};
 const resource={kind:'resource',id:'abc123',type:'WATER',status:'AVAILABLE',location};
 const other={...resource,id:'unrelated',type:'FOOD'};
 const candidate={kind:'request',id:'need',type:'WATER',status:'PENDING',location};
 const paths=[];
 const h=harness(undefined,false,async path=>{
  paths.push(path);
  if(path.startsWith('/map/locations'))return {items:[resource,other,candidate]};
  if(path.endsWith('/candidates'))return {items:[{...candidate,distanceMeters:20}]};
  if(path==='/requests/need')return {...candidate,details:'Clean drinking water'};
  return {};
 });
 await new Promise(setImmediate);
 assert.match(paths[0],/type=ALL/);
 h.get('map-record').change({target:{value:'resource/abc123'}});await new Promise(setImmediate);
 assert.deepEqual(h.groups.map(group=>group.items.length),[0,0,0,2]);
 const marker=h.groups[3].items[1];
 assert.match(marker.options.title,/Water/i);assert.match(marker.options.title,/request/i);
 marker.click();await new Promise(setImmediate);
 assert.equal(h.dialogs[0]?.record.id,'need');
 assert.equal(h.dialogs[0]?.opener,marker.getElement());
 assert.equal(h.dialogs[0]?.record.distanceMeters,20);
 assert.equal(h.views.length>0,true);
 h.get('map-record').change({target:{value:''}});
 assert.equal(h.groups[2].items.length,3);
 assert.equal(h.get('type').value,'WATER');
});
test('explicit overview fits its records once, while later saved-location polls preserve the view',async()=>{
 const location={latitude:60,longitude:-100},r={id:'away',kind:'resource',type:'FOOD',status:'AVAILABLE',location};
 let entries=[r];
 const h=harness(undefined,false,async path=>path.startsWith('/map/locations')?{items:entries}:{});
 await new Promise(setImmediate);
 assert.deepEqual(Array.from(h.views.at(-1)[0]),[60,-100]);
 const n=h.views.length;entries=[r,{...r,id:'new',location:{latitude:-20,longitude:90}}];
 await vm.runInContext('refreshLocations()',h.context);
 assert.equal(h.views.length,n);
});
test('a replaced map marker receives focus when the shared details dialog closes',async()=>{
 const resource={id:'saved',kind:'resource',type:'MEDICINE',status:'AVAILABLE',location:{latitude:2,longitude:3}};
 let status='AVAILABLE';
 const h=harness(undefined,false,async path=>path.startsWith('/map/locations')?{items:[{...resource,status}]}:{});
 await new Promise(setImmediate);
 const first=h.groups[2].items[0],views=h.views.length;
 first.click();
 assert.equal(h.dialogs.length,1);
 status='UNAVAILABLE';await vm.runInContext('refreshLocations()',h.context);
 const replacement=h.groups[2].items[0];assert.notEqual(replacement,first);
 h.dialogs[0].restoreFocus();
 assert.equal(replacement.getElement().focused,true);
 assert.equal(h.views.length,views);assert.equal(h.get('map-record').value,'');
});
