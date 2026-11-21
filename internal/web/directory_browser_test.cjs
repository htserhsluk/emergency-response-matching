const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const http=require('node:http');
const {chrome,until,launch}=require('./browser_fixture.cjs');

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
 const {evaluate,key,click,navigate}=await launch(t);
 await navigate('http://127.0.0.1:'+server.address().port+'/?tiles=off');
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
