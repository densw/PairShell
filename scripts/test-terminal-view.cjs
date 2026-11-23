const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const nodes = new Map();
const el = id => { if (!nodes.has(id)) nodes.set(id, {textContent:'',hidden:false}); return nodes.get(id); };
const terminals = [], requests = [], timers = [];
class Terminal {
 constructor(options) {this.options=options;this.writes=[];this.sizes=[];terminals.push(this);}
 open() {} reset() {this.writes=[];} resize(w,h) {this.sizes.push([w,h]);}
 write(bytes,done) {this.writes.push(Buffer.from(bytes).toString());done?.();}
}
let response = {events:[],cursor:0,width:80,height:24};
const context = vm.createContext({Terminal,Uint8Array,atob:value=>Buffer.from(value,'base64').toString('binary'),document:{querySelector:el},setTimeout:fn=>{timers.push(fn);return timers.length},clearTimeout(){},controlFetch:async(url,options)=>{requests.push({url,options});return {ok:true,json:async()=>response}},console});
vm.runInContext(fs.readFileSync('internal/uiassets/terminal-view.js','utf8'),context);
(async()=>{
 const mirror=context.PairShellMirror;
 assert.ok(mirror,'viewer available');
 mirror.setState('ACTIVE');
 response={events:[{sequence:1,type:'start',width:100,height:30},{sequence:2,type:'output',data:Buffer.from('\u001b[32mmacmini ~/Desktop % ls\r\n').toString('base64')}],cursor:2,width:100,height:30};
 await timers.shift()();
 const terminal=terminals[0];
 assert.equal(terminal.options.disableStdin,true,'customer terminal must be read only');
 assert.deepEqual(terminal.sizes[0],[100,30]);
 assert.ok(terminal.writes.join('').includes('~/Desktop % ls'));
 assert.equal(terminal.onData,undefined,'no terminal input handler');
 assert.equal(requests[0].url,'/api/terminal?after=0');
 response={events:[{sequence:3,type:'resize',width:120,height:40},{sequence:4,type:'output',data:Buffer.from('ok').toString('base64')}],cursor:4,width:120,height:40};
 await timers.shift()();
 assert.equal(requests[1].url,'/api/terminal?after=2');
 assert.deepEqual(terminal.sizes.at(-1),[120,40]);
 mirror.setState('PAUSED');
 response={events:[],cursor:4,width:120,height:40};
 await timers.shift()();
 assert.ok(el('#mirror-status').textContent.includes('暂停'),'pause keeps output visible');
 response={events:[{sequence:10,type:'output',data:Buffer.from('tail').toString('base64')}],cursor:10,width:120,height:40,truncated:true};
 await timers.shift()();
 assert.ok(el('#mirror-note').textContent.includes('最近'),'buffer gaps disclosed');
 assert.ok(terminal.writes.join('').includes('tail'));
 assert.ok(requests.every(r=>!r.options||!r.options.method||r.options.method==='GET'),'no write API');
 response={events:[{sequence:11,type:'end'}],cursor:11,width:120,height:40};
 await timers.shift()();
 mirror.setState('ACTIVE');
 assert.ok(el('#mirror-status').textContent.includes('已关闭'),'session polling must not claim a closed shell is running');
 response={events:[{sequence:12,type:'output',data:Buffer.from('FINAL_TAIL').toString('base64')},{sequence:13,type:'end'}],cursor:13,width:120,height:40};
 mirror.setState('ENDED');
 await timers.shift()();
 assert.ok(terminal.writes.join('').includes('FINAL_TAIL'),'end must drain the last output page');
 assert.ok(el('#mirror-status').textContent.includes('结束'));
 console.log('Customer terminal mirror: PASS (PTY bytes, size, read only, cursor, pause, gaps, end)');
})().catch(e=>{console.error(e);process.exitCode=1});
