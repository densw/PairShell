const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const tick=async()=>{for(let i=0;i<100;i++)await Promise.resolve()};
const pages=[[
 {sequence:1,type:'connection.started',data:'{"role":"merchant"}'},
 {sequence:2,type:'terminal.output',data:''},
 {sequence:3,type:'terminal.output',data:'\n\n'},
 {sequence:4,type:'terminal.output',data:'PS C:\\Users\\管理员> '}
],[
 {sequence:5,type:'terminal.output',data:'中文'}
],[
 {sequence:6,type:'terminal.output',data:'<script>alert(1)</script>\n'},
 {sequence:7,type:'session.end',data:'{"reason":"connection closed"}'}
],[]];
const want='协助连接已开始\n{"role":"merchant"}\n\nPS C:\\Users\\管理员> 中文<script>alert(1)</script>\n会话已结束\n{"reason":"connection closed"}\n\n';
function element(){return {value:'',checked:false,dataset:{},textContent:'',scrollTop:0,clientHeight:200,scrollHeight:100,showModal(){},close(){},replaceChildren(){},appendChild(){}}}
async function history(){
 const nodes=new Map(),el=id=>{if(!nodes.has(id))nodes.set(id,element());return nodes.get(id)};let selected,page=0;const urls=[];
 const dialog={open:false,querySelector:s=>el(s.match(/="(.*?)"/)[1]),showModal(){this.open=true},close(){this.open=false;this.onclose()}};
 const entry={id:'session-readable',kind:'session',title:'日志',created:'2026-11-27',note:''};
 const context=vm.createContext({URLSearchParams,location:{hash:'#token=fixture'},sessionStorage:{getItem(){}},document:{querySelector:()=>el('open'),createElement:t=>t==='dialog'?dialog:{},body:{appendChild(){}}},fetch:async url=>{urls.push(url);return {ok:true,json:async()=>url.startsWith('/history/events')?pages[page++]:[entry]}}});
 el('list').appendChild=b=>{selected=b};
 vm.runInContext(fs.readFileSync('internal/uiassets/history.js','utf8'),context);
 await el('open').onclick();await selected.onclick();await tick();
 assert.equal(el('events').textContent,want,'history must join chunks, suppress initial redraws and retain literal markup');
 assert.ok(urls.some(u=>u.endsWith('after=4')),'control-only records must still advance cursor');
 page=0;await selected.onclick();await tick();assert.equal(el('events').textContent,want,'reselect resets output and cursor');
}
async function control(endAction=false){
 const nodes=new Map(),el=id=>{if(!nodes.has(id))nodes.set(id,element());return nodes.get(id)};let page=0,state='ACTIVE';const urls=[],buttons=[{dataset:{action:'end'},disabled:false,hidden:false}];
 const context=vm.createContext({URL,URLSearchParams,document:{body:{dataset:{}},querySelector:el,querySelectorAll:()=>buttons},PairShellMirror:{setState(){}},location:{hash:''},history:{replaceState(){}},setInterval(){},clearInterval(){},fetch:async url=>{urls.push(url);if(url==='/api/end')return {ok:true,json:async()=>({state:'ENDED'})};return {ok:true,json:async()=>url==='/api/session'?{session:{state}}:url.startsWith('/api/events')?pages[page++]:[]}}});
 vm.runInContext(fs.readFileSync('internal/uiassets/control.js','utf8'),context);await tick();
 if(endAction)await buttons[0].onclick();else{state='ENDED';await vm.runInContext('refresh()',context)}await tick();
 assert.equal(el('#events').textContent,want,'live log must fetch final events and join chunks without raw event noise');
 assert.ok(urls.some(u=>u==='/api/events?after=4'));
 el('#show-records').onclick();el('#record-events').onclick();assert.equal(el('#record-page').textContent,want);
}
(async()=>{await history();await control();await control(true);console.log('Readable logs: PASS (continuous chunks, empty controls/cursors, Chinese, literal HTML, ended event, reselect reset)')})().catch(e=>{console.error(e);process.exitCode=1});
