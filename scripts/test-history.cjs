const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const tick=async()=>{for(let i=0;i<40;i++)await Promise.resolve()};
async function scenario(fail=false){
 const nodes=new Map(),calls=[];
 const el=id=>{if(!nodes.has(id))nodes.set(id,{value:'',checked:false,textContent:'',scrollTop:0,clientHeight:200,scrollHeight:100,replaceChildren(){},appendChild(){}});return nodes.get(id)};
 const dialog={open:false,querySelector:selector=>el(selector.match(/="(.*?)"/)[1]),showModal(){this.open=true},close(){this.open=false;this.onclose()}};
 const entry={id:'session-example',kind:'session',title:'test',note:'',created:'2026-01-01',deleted:false};
 let eventCalls=0;
 const context=vm.createContext({URLSearchParams,location:{hash:'#token=owner'},sessionStorage:{getItem(){}},document:{querySelector:()=>el('open'),createElement:tag=>tag==='dialog'?dialog:{},body:{appendChild(){}}},fetch:async(path,options)=>{
  calls.push({path,options});let body=[entry];
  if(path.startsWith('/history/events')){eventCalls++;if(fail)return {ok:false,text:async()=> 'read failed'};body=eventCalls===1?[{sequence:1,type:'terminal.output',data:'first large result'}]:eventCalls===2?[{sequence:2,type:'terminal.output',data:'second result'}]:[]}
  if(path==='/history/update')body={...entry,title:'changed'};
  if(path==='/history/delete')body={};
  return {ok:true,json:async()=>body};
 }});
 vm.runInContext(fs.readFileSync('internal/uiassets/history.js','utf8'),context);
 // List entry selection uses the generated button, so capture it through appendChild.
 let button;el('list').appendChild=b=>{button=b};
 await el('open').onclick();await button.onclick();await tick();
 if(fail){assert.equal(eventCalls,1,'read failure must stop automatic retries');assert.match(el('message').textContent,/read failed/);return}
 assert.equal(eventCalls,3,'short byte-limited responses must not end scrolling early');
 assert.match(el('events').textContent,/first large result/);assert.match(el('events').textContent,/second result/);
 el('title').value='changed';await el('save').onclick();assert.ok(calls.some(v=>v.path==='/history/update'&&JSON.parse(v.options.body).Title==='changed'));
 await el('delete').onclick();assert.ok(calls.some(v=>v.path==='/history/delete'&&JSON.parse(v.options.body).Deleted));
 dialog.close();
}
(async()=>{await scenario();await scenario(true);console.log('History: PASS (scroll all chunks, edit, trash, failed read stops retry)')})().catch(e=>{console.error(e);process.exitCode=1});
