const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');

function resourceHarness(){
 const elements=new Map(),waiting=[];
 const element=()=>({value:'',disabled:false,hidden:false,textContent:'',children:[],classList:{toggle(){}},append(...items){this.children.push(...items);},replaceChildren(...items){this.children=items;},showModal(){this.open=true;},focus(){},contains(){return false;},get firstElementChild(){return this.children[0];},reportValidity(){return true;},setAttribute(){},addEventListener(){}});
 const context=vm.createContext({document:{getElementById(id){if(!elements.has(id))elements.set(id,element());return elements.get(id);},createElement:element},crypto:{randomUUID:()=> 'fixture-operation-key'},AbortSignal:{timeout:()=>undefined},setTimeout(){},clearTimeout(){},fetch(path){return new Promise(resolve=>waiting.push({path,resolve}));}});
 vm.runInContext(fs.readFileSync(__dirname+'/app.js','utf8'),context);
 return {context,elements,waiting,reply(i,id){waiting[i].resolve({ok:true,json:async()=>({items:[{id,type:'WATER',status:'AVAILABLE',availability:true,location:{latitude:0,longitude:0}}]})});}};
}

test('resource refresh uses server-side unallocated filter and ignores stale responses',async()=>{
 const h=resourceHarness();
 const first=vm.runInContext('resources()',h.context),second=vm.runInContext('resources()',h.context);
 assert.match(h.waiting[0].path,/status=UNALLOCATED/);assert.match(h.waiting[0].path,/sort=newest/);
 h.reply(1,'new-result');await second;h.reply(0,'old-result');await first;
 assert.equal(h.elements.get('resource-list').children[0].children[1].textContent,'new-result');
});

test('resource refresh reports completion even if unchanged',async()=>{
 const h=resourceHarness();
 const first=vm.runInContext('resources()',h.context);h.reply(0,'same-result');await first;
 const second=vm.runInContext('resources()',h.context);h.reply(1,'same-result');await second;
 assert.match(h.elements.get('resource-feedback').textContent,/unchanged/i);
});

test('directory rows show only the readable type and resource ID',async()=>{
 const h=resourceHarness();
 const load=vm.runInContext('resources()',h.context);
 h.reply(0,'resource-one');
 await load;
 const row=h.elements.get('resource-list').children[0];
 assert.deepEqual(row.children.map(child=>child.textContent),['Water resource','resource-one']);
 assert.equal(typeof row.onclick,'function');
});

test('allocation history pages keep allocation sort and show known times in details',async()=>{
 const h=resourceHarness();
 h.elements.get('resource-filter').value='ALLOCATED';
 h.elements.get('resource-filter').onchange();
 const record={id:'history-id',type:'WATER',location:{latitude:0,longitude:1},availability:false,status:'ALLOCATED',assignedRequestId:'owner',allocatedAt:'2026-05-03T18:00:00Z',allocationConfirmedAt:'2026-05-03T18:01:00Z'};
 h.waiting[0].resolve({ok:true,json:async()=>({items:[record],next:'cursor/abc'})});
 await new Promise(setImmediate);
 const next=h.elements.get('more-resources').onclick();
 const url=new URL(h.waiting[1].path,'http://localhost');
 assert.deepEqual(Object.fromEntries(url.searchParams),{sort:'allocated',limit:'20',after:'cursor/abc',status:'ALLOCATED'});
 h.waiting[1].resolve({ok:true,json:async()=>({items:[record]})});
 await next;
 const reader=h.elements.get('resource-list').children[0].onclick();
 h.waiting[2].resolve({ok:true,json:async()=>record});
 await new Promise(setImmediate);
 assert.equal(h.waiting[3].path,'/requests/owner');
 const request={id:'owner',type:'WATER',details:'Needed water',status:'COMPLETED',location:{latitude:2,longitude:3},assignedResourceId:record.id,assignment:{resourceId:record.id,distanceMeters:0}};
 h.waiting[3].resolve({ok:true,json:async()=>request});
 await reader;
 const sections=h.elements.get('resource-dialog-fields').children;
 assert.deepEqual(sections.map(s=>s.children[0].textContent),['Resource','Matched request','Assignment']);
 const fields=sections.flatMap(s=>s.children[1].children.map(node=>node.textContent));
 for(const value of ['Allocated','Needed water','owner','0 metres'])assert.ok(fields.includes(value),value);
 assert.ok(!fields.includes('Allocation confirmed'));
 assert.equal(h.elements.get('resource-dialog-actions').children.length,0);
});

test('same request older poll cannot overwrite a newer completion response',async()=>{
 const h=resourceHarness();vm.runInContext("selected='request-one';notifications=async()=>{}",h.context);
 const older=vm.runInContext('refresh()',h.context),newer=vm.runInContext('refresh()',h.context);
 const reply=(i,status)=>h.waiting[i].resolve({ok:true,json:async()=>({id:'request-one',type:'WATER',status,details:'fixture',location:{latitude:0,longitude:0}})});
 reply(1,'COMPLETED');await newer;reply(0,'ASSIGNED');await older;
 assert.equal(h.elements.get('request-result').children[0].textContent,'COMPLETED');
 assert.equal(h.elements.get('complete').disabled,true);
});

test('created resource card updates availability and removes controls after allocation',async()=>{
 const h=resourceHarness();h.context.fixture={id:'created-one',type:'WATER',status:'AVAILABLE',availability:true,location:{latitude:0,longitude:0}};
 vm.runInContext('showCreatedResource(fixture)',h.context);
 const click=h.elements.get('created-resource').children[1].children.at(-1).onclick();
 const unavailable={...h.context.fixture,status:'UNAVAILABLE',availability:false};
 h.waiting[0].resolve({ok:true,json:async()=>unavailable});await new Promise(setImmediate);
 h.waiting[1].resolve({ok:true,json:async()=>unavailable});h.waiting[2].resolve({ok:true,json:async()=>({items:[]})});await click;
 assert.equal(h.elements.get('created-resource').children[1].children.at(-1).textContent,'Mark available');
 const refresh=vm.runInContext('refreshCreatedResource()',h.context);
 h.waiting[3].resolve({ok:true,json:async()=>({...unavailable,status:'ALLOCATED',assignedRequestId:'request-one'})});await refresh;
 const children=h.elements.get('created-resource').children[1].children;
 assert.match(children[0].textContent,/ALLOCATED/);assert.equal(children.at(-1).textContent,'Assigned request: request-one');
});

test('filter changes reset paging and suppress old filter results',async()=>{
 const h=resourceHarness();const old=vm.runInContext("resources('old-cursor')",h.context);
 h.elements.get('resource-filter').value='ALLOCATED';h.elements.get('resource-filter').onchange();
 assert.match(h.waiting[1].path,/after=&status=ALLOCATED/);
 assert.match(h.waiting[1].path,/sort=allocated/);
 h.reply(1,'history');await new Promise(setImmediate);h.reply(0,'old-unallocated');await old;
 assert.equal(h.elements.get('resource-list').children[0].children[1].textContent,'history');
});

test('repeating lookup during an in-flight refresh leaves one polling loop',async()=>{
 const elements=new Map(),timers=new Map(),waiting=[];let clockID=0;
 const element=()=>({value:'',disabled:false,hidden:false,textContent:'',classList:{toggle(){}},append(){},replaceChildren(){},reportValidity(){return true;}});
 const context=vm.createContext({
  document:{getElementById(id){if(!elements.has(id))elements.set(id,element());return elements.get(id);},createElement:element},
  crypto:{randomUUID:()=> 'fixture-operation-key'},AbortSignal:{timeout:()=>undefined},
  setTimeout(callback){const id=++clockID;timers.set(id,callback);return id;},clearTimeout(id){timers.delete(id);},
  fetch(path){if(path.includes('/notifications'))return Promise.resolve({ok:true,json:async()=>({items:[]})});return new Promise(resolve=>waiting.push(resolve));},
 });
 vm.runInContext(fs.readFileSync(__dirname+'/app.js','utf8'),context);
 const id='00000000000000000000000000000001';
 vm.runInContext(`follow('${id}');follow('${id}');`,context);
 assert.equal(waiting.length,2);
 for(const resolve of waiting)resolve({ok:true,json:async()=>({id,status:'PENDING',type:'WATER',details:'fixture',location:{latitude:0,longitude:0}})});
 await new Promise(setImmediate);await new Promise(setImmediate);
 assert.equal(timers.size,1,'old in-flight lookup must not schedule another permanent poll');
});

test('request entry retries a failed linked lookup and keeps newer records when older responses arrive',async()=>{
 const h=resourceHarness();
 const request={kind:'request',id:'first',type:'WATER',status:'ASSIGNED',assignedResourceId:'supply',assignment:{resourceId:'supply',distanceMeters:13},details:'Drinking water',location:{latitude:1,longitude:2}};
 h.context.request=request;
 const first=vm.runInContext("openRecordDetails(request,document.getElementById('open'))",h.context);
 h.waiting[0].resolve({ok:true,json:async()=>request});
 await new Promise(setImmediate);
 assert.equal(h.waiting[1].path,'/resources/supply');
 h.waiting[1].resolve({ok:false,status:503,json:async()=>({message:'Shard offline'})});
 await first;
 assert.equal(h.elements.get('record-detail-retry').hidden,false);
 assert.match(h.elements.get('resource-dialog-feedback').textContent,/resource/i);
 const retry=h.elements.get('record-detail-retry').onclick();
 await new Promise(setImmediate);
 h.waiting[2].resolve({ok:true,json:async()=>request});
 await new Promise(setImmediate);
 const later=vm.runInContext("openRecordDetails({kind:'request',id:'later',type:'FOOD'},document.getElementById('another'))",h.context);
 h.waiting[4].resolve({ok:true,json:async()=>({id:'later',type:'FOOD',status:'PENDING',details:'Newer request',location:{latitude:3,longitude:4}})});
 await later;
 h.waiting[3].resolve({ok:true,json:async()=>({id:'supply',type:'WATER',status:'ALLOCATED',assignedRequestId:'first',location:{latitude:0,longitude:0}})});
 await retry;
 const sections=h.elements.get('resource-dialog-fields').children;
 assert.deepEqual(sections.map(s=>s.children[0].textContent),['Request']);
 assert.ok(sections[0].children[1].children.some(n=>n.textContent==='Newer request'));
});

test('failed history lookup still checks its linked request, and Retry keeps confirmed fields visible',async()=>{
 const h=resourceHarness(),record={kind:'resource',id:'supply',type:'WATER',status:'ALLOCATED',assignedRequestId:'owner',allocationConfirmedAt:'2026-05-03T18:01:00Z'};
 const req={id:'owner',type:'WATER',status:'ASSIGNED',assignedResourceId:'supply',details:'Keep this description',assignment:{resourceId:'supply',distanceMeters:0}};
 h.context.record=record;
 const opened=vm.runInContext("openRecordDetails(record,document.getElementById('open'))",h.context);
 h.waiting[0].resolve({ok:false,status:503,json:async()=>({message:'Shard unavailable'})});
 await new Promise(setImmediate);
 assert.equal(h.waiting[1]?.path,'/requests/owner');
 h.waiting[1].resolve({ok:true,json:async()=>req});await opened;
 function all(){return h.elements.get('resource-dialog-fields').children.flatMap(s=>s.children[1].children.map(n=>n.textContent));}
 assert.ok(all().includes('Keep this description'));
 assert.ok(all().includes('Allocation confirmed'));
 assert.ok(!all().includes('Allocated'));
 const retry=h.elements.get('record-detail-retry').onclick();
 assert.ok(all().includes('Keep this description'));
 h.waiting[2].resolve({ok:true,json:async()=>record});await new Promise(setImmediate);
 h.waiting[3].resolve({ok:false,status:503,json:async()=>({message:'Request unavailable'})});await retry;
 assert.ok(all().includes('Keep this description'));
 assert.ok(all().includes('0 metres'));
 assert.equal(h.elements.get('record-detail-retry').hidden,false);
});
