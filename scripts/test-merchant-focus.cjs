const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const elements=new Map(),sockets=[];
let state={Phase:'idle',CanConnect:true,CanEnd:false,AttemptID:0},fitCount=0;
const element=id=>{if(!elements.has(id))elements.set(id,{value:'',textContent:'',disabled:false,hidden:false,dataset:{}});return elements.get(id)};
class Socket{static OPEN=1;constructor(){this.readyState=0;sockets.push(this)}send(value){(this.sent??=[]).push(JSON.parse(value))}close(){this.readyState=3;this.onclose?.()}open(){this.readyState=1;this.onopen?.()}ready(attempt){this.onmessage({data:JSON.stringify({ready:true,attempt_id:attempt})})}}
class Terminal{constructor(){this.cols=80;this.rows=24}loadAddon(){}open(){}write(){}writeln(){}onData(){}onResize(){}attachCustomKeyEventHandler(){}reset(){}focus(){}getSelection(){return ''}paste(){}}
const context=vm.createContext({URLSearchParams,TextEncoder,Uint8Array,ArrayBuffer,console,Terminal,FitAddon:{FitAddon:class{fit(){fitCount++}}},ResizeObserver:class{observe(){}},WebSocket:Socket,document:{querySelector:element},history:{replaceState(){}},location:{hash:'#token=local',protocol:'http:',host:'127.0.0.1:1234'},sessionStorage:{getItem(){return null},setItem(){}},navigator:{clipboard:{}},setInterval(){},fetch:async url=>({ok:true,json:async()=>url.endsWith('/state')?state:{}})});
vm.runInContext(fs.readFileSync(path.join(__dirname,'../internal/uiassets/merchant.js'),'utf8'),context);
const run=code=>vm.runInContext(code,context);
async function settle(){for(let i=0;i<30;i++)await Promise.resolve()}
async function refresh(value){state={...state,...value};await run('refresh()')}
(async()=>{
 await settle();
 assert.equal(element('#toggle-connection').hidden,true);
 await refresh({Phase:'active',CanConnect:false,CanEnd:true,AttemptID:1,Route:{selected:'direct',rtt_ms:3}});
 assert.equal(element('.connection-pane').hidden,false,'form stays visible while PTY opens');
 const ws=sockets.at(-1);ws.open();ws.ready(1);
 assert.equal(element('.connection-pane').hidden,true,'ready PTY automatically gets full width');
 assert.equal(element('.workspace').dataset.pane,'closed');
 assert.equal(element('#toggle-connection').ariaExpanded,'false');
 assert.match(element('#route-status').textContent,/直连.*3.0 ms.*流畅/);
 assert.equal(element('#route-status').dataset.quality,'good');
 const before=fitCount;element('#toggle-connection').onclick();
 assert.equal(element('.connection-pane').hidden,false);assert.ok(fitCount>before,'toggle refits terminal');
 assert.equal(ws.sent.at(-1).type,'resize','toggle sends actual new terminal dimensions');
 await refresh({Route:{selected:'relay',rtt_ms:80}});
 assert.equal(element('.connection-pane').hidden,false,'poll does not override manual panel choice');
 assert.equal(element('#route-status').dataset.quality,'fair');
 assert.match(element('#route-status').textContent,/中继.*稍慢/);
 await refresh({Route:{selected:'relay',rtt_ms:200}});assert.equal(element('#route-status').dataset.quality,'fair');
 await refresh({Route:{selected:'relay',rtt_ms:201}});assert.equal(element('#route-status').dataset.quality,'poor');assert.match(element('#route-status').textContent,/高延迟/);
 await refresh({Route:{selected:'direct',rtt_ms:0}});assert.equal(element('#route-status').dataset.quality,'unknown');assert.match(element('#route-status').textContent,/测量中/);
 element('#toggle-connection').onclick();assert.equal(element('.connection-pane').hidden,true);
 await refresh({Phase:'ended',CanConnect:true,CanEnd:false});
 assert.equal(element('.connection-pane').hidden,false,'ending returns to connection form');assert.equal(element('#route-status').hidden,true);
 await refresh({Phase:'active',CanConnect:false,CanEnd:true,AttemptID:2});const next=sockets.at(-1);next.open();
 ws.ready(1);assert.equal(element('.connection-pane').hidden,false,'old ready cannot collapse new attempt');
 next.ready(2);assert.equal(element('.connection-pane').hidden,true);
 await refresh({Phase:'error',CanConnect:true,CanEnd:false});assert.equal(element('.connection-pane').hidden,false,'error returns to connection form');
 const html=fs.readFileSync(path.join(__dirname,'../internal/uiassets/merchant.html'),'utf8');
 assert.equal((html.match(/id="route-status"/g)||[]).length,1);
 assert.ok(html.indexOf('id="route-status"')<html.indexOf('</header>'),'route belongs beside header connection state');
 console.log('Merchant focus UI: PASS (ready collapse, manual panel, retry restore, stale ready, RTT levels)');
})().catch(error=>{console.error(error);process.exitCode=1});
