const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
function fixture(script){
 const elements=new Map(),sockets=[],calls=[];let state={Phase:'idle',CanConnect:true,CanEnd:false,AttemptID:0};
 const element=id=>{if(!elements.has(id))elements.set(id,{value:'',textContent:'',disabled:false,hidden:false,dataset:{},open:false});return elements.get(id)};
 class Socket{static OPEN=1;constructor(url){this.url=url;this.readyState=0;sockets.push(this)}send(value){(this.sent??=[]).push(JSON.parse(value))}close(){this.readyState=3;this.onclose?.()}open(){this.readyState=1;this.onopen?.()}}
 class Terminal{constructor(){this.cols=80;this.rows=24}loadAddon(){}open(){}write(){}writeln(){}onData(fn){this.input=fn}onResize(){}attachCustomKeyEventHandler(){}reset(){}focus(){}getSelection(){return ''}paste(){}}
 const context=vm.createContext({URLSearchParams,TextEncoder,Uint8Array,ArrayBuffer,console,Terminal,FitAddon:{FitAddon:class{fit(){}}},ResizeObserver:class{observe(){}},WebSocket:Socket,document:{querySelector:element},history:{replaceState(){}},location:{hash:'#token=window-capability',protocol:'http:',host:'127.0.0.1:1234',assign(url){calls.push({navigation:url})}},sessionStorage:{getItem(){return null},setItem(){}},navigator:{clipboard:{writeText:async()=>{},readText:async()=>''}},setInterval(){},setTimeout(resolve){resolve()},fetch:async(url,options)=>{calls.push({url,options});const ok=!context.rejectJoin||url!=='/merchant/join';return {ok,status:ok?200:409,text:async()=> '配对码已失效，请刷新后重试',json:async()=> url.endsWith('/state')||url==='/status'?state:url.endsWith('/settings')?{Server:'https://relay.example',server:'https://relay.example'}:{}}}});
 vm.runInContext(fs.readFileSync(path.join(__dirname,'../internal/uiassets/',script),'utf8'),context);
 return {context,e:element,sockets,calls,setState(value){state=value},run(code){return vm.runInContext(code,context)}};
}
async function settle(){for(let i=0;i<40;i++)await Promise.resolve()}
(async()=>{
 const f=fixture('merchant.js');await settle();
 f.setState({Phase:'error',CanConnect:true,CanEnd:false,AttemptID:1,State:'配对未完成，请确认配对码有效及服务器支持短码配对'});await f.run('refresh()');
 assert.equal(f.e('#connect').disabled,false,'failure must allow retry');assert.equal(f.e('#end').disabled,true);assert.equal(f.e('#merchant-state').textContent,'连接未完成');assert.match(f.e('#merchant-error').textContent,/配对未完成/);
 f.e('#pair-code').value='1234-5678-9012';f.context.rejectJoin=true;await f.e('#connect-form').onsubmit({preventDefault(){}});assert.equal(f.e('#pair-code').value,'123456789012','failure must retain editable code');assert.equal(f.e('#connect').disabled,false);
 f.setState({Phase:'active',CanConnect:false,CanEnd:true,AttemptID:2,State:'已连接'});await f.run('refresh()');assert.equal(f.sockets.length,1);const old=f.sockets[0];old.open();assert.equal(old.sent[0].attempt_id,2);f.run("term.input('before-ready')");assert.equal(old.sent.length,1,'input must wait for PTY acceptance');old.onmessage({data:JSON.stringify({ready:true,attempt_id:2})});f.run("term.input('after-ready')");assert.equal(old.sent.at(-1).type,'input');assert.equal(old.sent.at(-1).data,'after-ready');await f.run('refresh()');assert.equal(f.sockets.length,1,'polling must not create extra shells');
 f.setState({Phase:'active',CanConnect:false,CanEnd:true,AttemptID:3,State:'已连接'});await f.run('refresh()');const current=f.sockets[1];current.open();old.onclose();assert.equal(f.e('#reopen-terminal').hidden,true,'old socket close must not affect new terminal');
 current.close();assert.equal(f.e('#reopen-terminal').hidden,false);await f.run('refresh()');assert.equal(f.sockets.length,2,'disconnected shell must not silently reopen');f.e('#reopen-terminal').onclick();assert.equal(f.sockets.length,3,'explicit reopen creates a new terminal');
 f.setState({Phase:'ended',CanConnect:true,CanEnd:false,AttemptID:3,State:'已结束'});await f.run('refresh()');assert.equal(f.e('#connect').disabled,false,'ended session must allow second connection');
 const c=fixture('desktop.js');await settle();c.setState({Phase:'error',CanGenerate:true,CanReset:false,AttemptID:1,State:'配对码已过期'});await c.run('status()');assert.equal(c.e('#copy-code').disabled,true);assert.equal(c.e('#regenerate-code').disabled,false);assert.equal(c.e('#regenerate-code').textContent,'重新生成配对码');
 c.setState({Phase:'waiting',CanGenerate:true,CanReset:true,AttemptID:2,PairCode:'1234-5678-9012'});await c.run('status()');assert.equal(c.e('#copy-code').disabled,false);assert.equal(c.e('#regenerate-code').textContent,'刷新配对码');
 console.log('UI reconnect behavior: PASS (retry, stale socket, explicit reopen, customer regeneration)');
})().catch(error=>{console.error(error);process.exitCode=1});
