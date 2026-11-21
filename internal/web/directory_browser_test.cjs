const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const os=require('node:os');
const http=require('node:http');
const {spawn}=require('node:child_process');
const {setTimeout:delay}=require('node:timers/promises');

const chrome=[process.env.CHROME_BIN,'C:/Program Files/Google/Chrome/Application/chrome.exe','/usr/bin/chromium','/usr/bin/google-chrome'].find(file=>file&&fs.existsSync(file));
async function until(check){
 const deadline=Date.now()+15000;
 while(Date.now()<deadline){const value=await check();if(value)return value;await delay(50);}
 throw Error('Browser condition timed out: '+check);
}

test('directory dialog supports native keyboard, mouse, availability and focus restoration',{skip:!chrome,timeout:60000},async t=>{
 const id='a'.repeat(32);
 let record={id,type:'WATER',availability:true,status:'AVAILABLE',registeredAt:'2026-07-06T21:00:00Z',location:{latitude:0,longitude:-13}};
 const assets=new Map([['/','index.html'],['/app.js','app.js'],['/styles.css','styles.css']]);
 const server=http.createServer(async(req,res)=>{
  const url=new URL(req.url,'http://localhost');
  if(assets.has(url.pathname)){
   res.setHeader('content-type',url.pathname==='/'?'text/html':url.pathname.endsWith('.js')?'text/javascript':'text/css');
   res.end(fs.readFileSync(path.join(__dirname,assets.get(url.pathname))));
   return;
  }
  if(url.pathname==='/resources/'+id+'/status'&&req.method==='PUT'){
   const chunks=[];for await(const chunk of req)chunks.push(chunk);
   const {status}=JSON.parse(Buffer.concat(chunks));
   record={...record,status,availability:status==='AVAILABLE'};
   res.end(JSON.stringify(record));
  }else if(url.pathname==='/resources/'+id){res.end(JSON.stringify(record));}
  else if(url.pathname==='/resources'){res.end(JSON.stringify({items:[{...record,location:{latitude:1,longitude:-10}}]}));}
  else {res.writeHead(404);res.end();}
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 t.after(()=>{server.closeAllConnections();server.close();});
 const profile=fs.mkdtempSync(path.join(os.tmpdir(),'erm-directory-'));
 const browser=spawn(chrome,['--headless','--disable-background-networking','--disable-extensions','--disable-gpu','--no-first-run','--remote-debugging-port=0','--user-data-dir='+profile,'about:blank'],{stdio:'ignore',windowsHide:true});
 const closed=new Promise(resolve=>browser.once('exit',resolve));
 t.after(async()=>{
  browser.kill();await closed;
  try{fs.rmSync(profile,{recursive:true,force:true,maxRetries:3});}catch(error){if(!['EPERM','EBUSY','ENOTEMPTY'].includes(error.code))throw error;}
 });
 const port=await until(()=>{
  try{return fs.readFileSync(path.join(profile,'DevToolsActivePort'),'utf8').split('\n')[0];}
  catch(error){if(!['ENOENT','EBUSY'].includes(error.code))throw error;return false;}
 });
 const pages=await (await fetch('http://127.0.0.1:'+port+'/json/list')).json();
 const socket=new WebSocket(pages.find(p=>p.type==='page').webSocketDebuggerUrl);
 await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject;});
 t.after(()=>socket.close());
 let sequence=0;
 const pending=new Map();
 socket.onmessage=event=>{
  const reply=JSON.parse(event.data),wait=pending.get(reply.id);
  if(wait){pending.delete(reply.id);reply.error?wait.reject(Error(reply.error.message)):wait.resolve(reply.result);}
 };
 function cdp(method,params={}){
  const id=++sequence;
  return new Promise((resolve,reject)=>{pending.set(id,{resolve,reject});socket.send(JSON.stringify({id,method,params}));});
 }
 async function evaluate(expression){
  const result=await cdp('Runtime.evaluate',{expression,returnByValue:true});
  if(result.exceptionDetails)throw Error(result.exceptionDetails.exception.description);
  return result.result.value;
 }
 async function key(key,code,number){
  for(const type of ['keyDown','keyUp'])await cdp('Input.dispatchKeyEvent',{type,key,code,windowsVirtualKeyCode:number,text:type==='keyDown'&&key==='Enter'?'\r':''});
 }
 async function click(selector){
  const point=await evaluate(`(()=>{const node=document.querySelector(${JSON.stringify(selector)});node.scrollIntoView({block:'center'});const rect=node.getBoundingClientRect();return {x:rect.x+rect.width/2,y:rect.y+rect.height/2};})()`);
  for(const type of ['mousePressed','mouseReleased'])await cdp('Input.dispatchMouseEvent',{type,...point,button:'left',clickCount:1});
 }
 await cdp('Page.navigate',{url:'http://127.0.0.1:'+server.address().port+'/?tiles=off'});
 await until(()=>evaluate("typeof resources==='function'"));
 await click('#resources');
 await until(()=>evaluate("!!document.querySelector('.resource-entry')"));
 await evaluate("document.querySelector('.resource-entry').focus()");
 await key('Enter','Enter',13);
 await until(()=>evaluate("document.querySelector('dialog').open"));
 await until(()=>evaluate("document.querySelector('#resource-dialog-feedback').textContent===''"));
 assert.equal(await evaluate('document.activeElement.id'),'resource-dialog-close');
 assert.match(await evaluate("document.querySelector('dialog').innerText"),/Water resource[\s\S]+AVAILABLE[\s\S]+Longitude[\s\S]+-13/);
 await key('Escape','Escape',27);
 await until(()=>evaluate("!document.querySelector('dialog').open && document.activeElement.className==='resource-entry'"));
 await click('.resource-entry');
 await until(()=>evaluate("!!document.querySelector('#resource-dialog-actions button:enabled')"));
 await click('#resource-dialog-actions button');
 await until(()=>evaluate("document.querySelector('#resource-dialog-feedback').textContent.includes('updated') && document.querySelector('#resource-dialog-actions button').textContent==='Mark available'"));
 assert.equal(record.status,'UNAVAILABLE');
 await click('#resource-dialog-close');
 await until(()=>evaluate("!document.querySelector('dialog').open && document.activeElement.className==='resource-entry'"));
});
