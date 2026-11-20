const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');

function resourceHarness(){
 const elements=new Map(),waiting=[];
 const element=()=>({value:'',disabled:false,hidden:false,textContent:'',children:[],classList:{toggle(){}},append(...items){this.children.push(...items);},replaceChildren(...items){this.children=items;},reportValidity(){return true;},setAttribute(){},addEventListener(){}});
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
