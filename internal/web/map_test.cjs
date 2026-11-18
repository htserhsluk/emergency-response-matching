const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
function harness(geo,tiles=false){
 const elements=new Map(),groups=[],views=[];let resize;const tileEvents={};
 const element=()=>({value:'',textContent:'',disabled:false,addEventListener(name,fn){this[name]=fn;}});
 const get=id=>{if(!elements.has(id))elements.set(id,element());return elements.get(id);};
 get('latitude').value='12.9';get('longitude').value='77.6';
 const map={setView(p){views.push(p);return this;},fitBounds(p){views.push(p);return this;},on(name,fn){this[name]=fn;return this;},invalidateSize(){}};
 const marker=point=>({point,bindTooltip(){return this;},addTo(g){g.items.push(this);return this;}});
 const L={tileLayer(){return {addTo(){return this;},on(event,fn){tileEvents[event]=fn;return this;}};},map:()=>map,layerGroup(){const g={items:[],addTo(){groups.push(this);return this;},clearLayers(){this.items=[];}};return g;},marker,circleMarker:marker,polyline:marker,divIcon:o=>o,latLngBounds:p=>p};
 const context=vm.createContext({window:{L},L,document:{getElementById:get,createElement:element},navigator:geo?{geolocation:geo}:{},location:{search:tiles?'':'?tiles=off'},URLSearchParams,ResizeObserver:class{constructor(fn){resize=fn;}observe(){}},api:async()=>tiles?({tileUrl:"https://example.invalid/{z}/{x}/{y}.png",attribution:"Fixture tiles"}):({}),locationInput:()=>({latitude:Number(get('latitude').value),longitude:Number(get('longitude').value)}),message(){},setTimeout:fn=>fn()});
 vm.runInContext(fs.readFileSync(__dirname+'/map.js','utf8'),context);
 return {context,get,groups,views,map,tileEvents,resize:()=>resize()};
}
test('map draws initial selection even with tiles disabled',()=>{
 const h=harness();assert.equal(h.groups[0].items.length,1);
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
 h.context.fixture=r;vm.runInContext('drawAssignment(fixture)',h.context);assert.equal(h.views.at(-1).length,2);assert.ok(Array.isArray(h.views.at(-1)[0]),'fit both points');const n=h.views.length;
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
 vm.runInContext('drawAssignment(fixture);clearMappedRequest()',h.context);assert.equal(h.groups[1].items.length,0);
});

test('temporary missing resource coordinates do not reset the users map view when polling recovers',()=>{
 const h=harness();h.context.fixture={id:'one',location:{latitude:0,longitude:0},assignment:{resourceId:'two',location:{latitude:1,longitude:1}}};
 vm.runInContext('drawAssignment(fixture)',h.context);const n=h.views.length;
 vm.runInContext('drawAssignment({...fixture,assignment:{resourceId:"two"}});drawAssignment(fixture)',h.context);
 assert.equal(h.views.length,n);
});

test('tile failure and resize preserve usable selection',async()=>{
 const h=harness(undefined,true);await new Promise(setImmediate);
 h.tileEvents.tileerror();h.resize();
 assert.match(h.get('map-note').textContent,/tiles unavailable/);assert.equal(h.groups[0].items.length,1);
});
