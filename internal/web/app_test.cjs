const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');

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
