const fs=require('node:fs');
const path=require('node:path');
const os=require('node:os');
const {spawn}=require('node:child_process');
const {setTimeout:delay}=require('node:timers/promises');

const chrome=[process.env.CHROME_BIN,'C:/Program Files/Google/Chrome/Application/chrome.exe','C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe','/usr/bin/google-chrome','/usr/bin/chromium'].find(file=>file&&fs.existsSync(file));
async function until(check){
 const deadline=Date.now()+15000;
 while(Date.now()<deadline){const value=await check();if(value)return value;await delay(50);}
 throw Error('Browser condition timed out: '+check);
}

async function launch(t){
 const profile=fs.mkdtempSync(path.join(os.tmpdir(),'erm-web-test-'));
 const browser=spawn(chrome,['--headless','--disable-background-networking','--disable-extensions','--disable-gpu','--no-first-run','--remote-debugging-port=0','--user-data-dir='+profile,'about:blank'],{stdio:['ignore','ignore','pipe'],windowsHide:true});
 const closed=new Promise(resolve=>browser.once('close',resolve));
 let diagnostics='';
 browser.stderr.setEncoding('utf8');
 const endpoint=new Promise((resolve,reject)=>{
  const timer=setTimeout(()=>reject(Error('Chromium startup timed out: '+diagnostics)),30000);
  function finish(error,port){
   clearTimeout(timer);
   if(error)reject(error);else resolve(port);
  }
  browser.on('error',error=>finish(error));
  browser.once('close',(code,signal)=>finish(Error('Chromium exited during startup ('+(signal??code)+'): '+diagnostics)));
  browser.stderr.on('data',chunk=>{
   diagnostics=(diagnostics+chunk).slice(-8192);
   const listening=diagnostics.match(/DevTools listening on ws:\/\/127\.0\.0\.1:(\d+)\//);
   if(listening)finish(null,listening[1]);
  });
 });
 t.after(async()=>{
  browser.kill();await closed;
  try{fs.rmSync(profile,{recursive:true,force:true,maxRetries:3});}catch(error){if(!['EPERM','EBUSY','ENOTEMPTY'].includes(error.code))throw error;}
 });
 const port=await endpoint;
 const page=await until(async()=>{
  const response=await fetch('http://127.0.0.1:'+port+'/json/list');
  return (await response.json()).find(item=>item.type==='page'&&item.webSocketDebuggerUrl);
 });
 const socket=new WebSocket(page.webSocketDebuggerUrl);
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
 return {evaluate,key,click,navigate:url=>cdp('Page.navigate',{url})};
}
module.exports={chrome,until,launch};
