const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const {chrome, until, launch} = require('./browser_fixture.cjs');

test('map and directory share complete allocation details, recovery and native focus', {skip: !chrome, timeout: 80000}, async t => {
 const allocated = {
  id: 'a'.repeat(32), type: 'WATER', status: 'ALLOCATED', availability: false,
  registeredAt: '2026-11-10T11:12:13Z', allocatedAt: '2026-11-21T09:10:11Z',
  location: {latitude: 12.9123, longitude: 77.6123}, assignedRequestId: 'b'.repeat(32)
 };
 const matched = {
  id: allocated.assignedRequestId, type: 'WATER', status: 'COMPLETED', generation: 3,
  details: 'Bottled water for the clinic', createdAt: '2026-11-19T10:11:12Z',
  location: {latitude: 12.9321, longitude: 77.5987}, assignedResourceId: allocated.id,
  assignment: {resourceId: allocated.id, location: allocated.location, distanceMeters: 1847.125}
 };
 const available = {id: 'c'.repeat(32), type: 'MEDICINE', status: 'AVAILABLE', availability: true,
  location: {latitude: 13.2, longitude: 77.1}, registeredAt: '2026-11-11T13:14:15Z'};
 const legacy = {...allocated, id: 'd'.repeat(32), type: 'FOOD', assignedRequestId: '1'.repeat(32),
  location: {latitude: 12.8, longitude: 77.8}, allocatedAt: undefined, allocationConfirmedAt: '2026-11-15T13:14:15Z'};
 const undated = {...legacy, id: 'e'.repeat(32), type: 'SHELTER', assignedRequestId: '2'.repeat(32),
  location: {latitude: 12.7, longitude: 77.4}, registeredAt: undefined, allocationConfirmedAt: undefined};
 const resources = [allocated, available, legacy, undated];
 const requests = [matched, ...[legacy,undated].map(r => ({...matched, id: r.assignedRequestId, type: r.type,
  assignedResourceId: r.id, assignment: {...matched.assignment, resourceId: r.id, location: r.location}}))];
 const assets = new Set(['/index.html','/styles.css','/app.js','/map.js','/vendor/leaflet/leaflet.js','/vendor/leaflet/leaflet.css']);
 let requestFailure = false;
 const server = http.createServer((req, res) => {
  const url = new URL(req.url,'http://localhost');
  const route = url.pathname === '/' ? '/index.html' : url.pathname;
  if (assets.has(route)) {
   res.setHeader('content-type', route.endsWith('.css')?'text/css':route.endsWith('.html')?'text/html':'text/javascript');
   res.end(fs.readFileSync(path.join(__dirname,route.slice(1))));
   return;
  }
  res.setHeader('content-type','application/json');
  let body;
  if (route === '/map/locations') {
   body = {items: resources.map(r => ({kind:'resource',id:r.id,type:r.type,status:r.status,location:r.location}))};
  } else if (route === '/resources') {
   body = {items:resources.filter(r => url.searchParams.get('status') === 'ALLOCATED' ? !r.availability : r.availability)};
  } else if (route.endsWith('/candidates')) {
   const reqRecord = requests.find(r => route === '/resources/'+r.assignedResourceId+'/candidates');
   body = {items:[], assignedRequest:reqRecord && {id:reqRecord.id,location:reqRecord.location,distanceMeters:reqRecord.assignment.distanceMeters}};
  } else if (route === '/web-config') {
   body = {tileUrl:''};
  } else if (route === '/requests/'+matched.id && requestFailure) {
   res.statusCode=503;
   body={message:'Linked request temporarily unavailable'};
  } else {
   body = resources.find(r=>route==='/resources/'+r.id) || requests.find(r=>route==='/requests/'+r.id);
   if (!body){res.statusCode=404;body={message:'Fixture route unavailable'};}
  }
  res.end(JSON.stringify(body));
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 t.after(()=>{server.closeAllConnections();server.close();});
 const {evaluate,click,key,navigate} = await launch(t);
 const close = '#resource-dialog-close';
 const pin = '.location-pin[title="Water resource"]';
 const entry = r => '#resource-entry-'+r.id;
 const waitFor = expression => until(()=>evaluate(expression));
 const sectionTexts = () => evaluate(`[...document.querySelectorAll('#resource-dialog-fields section')].map(section=>({
  heading:section.querySelector('h3').innerText, text:section.innerText
 }))`);
 const dialogText = () => evaluate("document.getElementById('resource-dialog').innerText");
 const locale = iso => evaluate('new Date('+JSON.stringify(iso)+').toLocaleString()');
 async function expectFocus(selector) {
  await waitFor('document.activeElement?.matches('+JSON.stringify(selector)+')');
 }
 async function chooseHistory(value){
  await evaluate(`(()=>{const filter=document.getElementById('resource-filter');filter.value=${JSON.stringify(value)};filter.dispatchEvent(new Event('change'));})()`);
 }
 async function openRow(r) {
  await waitFor('!!document.querySelector('+JSON.stringify(entry(r))+')');
  await click(entry(r));
  await waitFor("document.getElementById('resource-dialog').open");
 }
 async function closeAndCheck(selector, escape=false) {
  if(escape)await key('Escape','Escape',27);else await click(close);
  await waitFor("!document.getElementById('resource-dialog').open");
  await expectFocus(selector);
 }
 async function waitForDetails(text) {
  await waitFor("document.getElementById('resource-dialog').innerText.includes("+JSON.stringify(text)+")");
 }

 await navigate('http://127.0.0.1:'+server.address().port+'/?tiles=off');
 await waitFor('!!document.querySelector('+JSON.stringify(pin)+')');
 await click(pin);
 await waitForDetails(matched.details);
 await expectFocus(close);
 const fromPin = await sectionTexts();
 assert.deepEqual(fromPin.map(s=>s.heading),['Resource','Matched request','Assignment']);
 const fields = section => evaluate(`Object.fromEntries([...document.querySelectorAll('#resource-dialog-fields section[data-detail="${section}"] dt')].map(label => [label.innerText, label.nextElementSibling.innerText]))`);
 assert.deepEqual(await fields('resource'),{
  'Resource ID':allocated.id,'Assistance type':'WATER',Status:'ALLOCATED','Available for matching':'No',
  Latitude:'12.9123',Longitude:'77.6123',Registered:await locale(allocated.registeredAt)
 });
 assert.deepEqual(await fields('request'),{
  'Request ID':matched.id,Description:matched.details,'Assistance type':'WATER',Status:'COMPLETED',
  Latitude:'12.9321',Longitude:'77.5987',Created:await locale(matched.createdAt),Generation:'3'
 });
 assert.deepEqual(await fields('assignment'),{
  'Assigned resource ID':allocated.id,'Matched request ID':matched.id,'Saved distance':'1847.125 metres',
  Allocated:await locale(allocated.allocatedAt)
 });
 await closeAndCheck(pin,true);
 await chooseHistory('ALLOCATED');
 await openRow(allocated);
 await waitForDetails(matched.details);
 assert.deepEqual(await sectionTexts(),fromPin,'The directory and map must expose identical evidence');
 await closeAndCheck(entry(allocated));

 requestFailure=true;
 await openRow(allocated);
 await waitFor("!!document.querySelector('#record-detail-retry:not([hidden])')");
 assert.ok((await dialogText()).includes(allocated.id),'Resource details should survive a failed request lookup');
 requestFailure=false;
 await click('#record-detail-retry');
 await waitForDetails(matched.details);
 assert.deepEqual(await sectionTexts(),fromPin,'Retry must restore the complete details');
 await closeAndCheck(entry(allocated),true);

 await openRow(legacy);
 await waitForDetails(await locale(legacy.allocationConfirmedAt));
 await waitForDetails(matched.details);
 assert.equal((await fields('assignment')).Allocated,undefined);
 assert.equal((await fields('assignment'))['Allocation confirmed'],await locale(legacy.allocationConfirmedAt));
 await closeAndCheck(entry(legacy));
 await openRow(undated);
 await waitForDetails(matched.details);
 assert.equal((await fields('resource')).Registered,'Not recorded');
 const undatedFields=await fields('assignment');
 assert.equal(undatedFields['Allocation time'],'Not recorded');
 assert.equal(undatedFields.Allocated,undefined);
 assert.equal(undatedFields['Allocation confirmed'],undefined);
 await closeAndCheck(entry(undated));
 await chooseHistory('UNALLOCATED');
 await openRow(available);
 await waitForDetails(await locale(available.registeredAt));
 assert.deepEqual((await sectionTexts()).map(s=>s.heading),['Resource']);
 assert.match(await dialogText(),/AVAILABLE/);
 await closeAndCheck(entry(available),true);
});
