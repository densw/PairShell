const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const nodes=new Map(),timers=[];let bounds={width:800,height:300},observer;
const screen={clientWidth:640,clientHeight:384},style={};let instance;
const el=id=>{if(!nodes.has(id))nodes.set(id,{textContent:'',hidden:false,getBoundingClientRect:()=>bounds});return nodes.get(id)};
class Terminal{
 constructor(options){this.options=options;this.cols=80;this.rows=24;this.element={style,querySelector:()=>screen};instance=this}
 open(){}reset(){}write(data,done){done()}
 resize(cols,rows){this.cols=cols;this.rows=rows;screen.clientWidth=cols*8;screen.clientHeight=rows*16}
}
const ctx=vm.createContext({Terminal,Uint8Array,atob:()=>'',ResizeObserver:class{constructor(fn){observer=fn}observe(){}},document:{querySelector:el},setTimeout:fn=>timers.push(fn),clearTimeout(){},controlFetch:async()=>({ok:true,json:async()=>({events:[{sequence:1,type:'start',width:100,height:30}],cursor:1})})});
vm.runInContext(fs.readFileSync('internal/uiassets/terminal-view.js','utf8'),ctx);
(async()=>{
 ctx.PairShellMirror.setState('ACTIVE');await timers.shift()();
 assert.equal(instance.cols,100);assert.equal(instance.rows,30,'viewer must preserve the remote PTY grid');
 const scale=Number(style.transform.match(/[\d.]+/)[0]);
 assert.ok(screen.clientHeight*scale<=bounds.height);assert.ok(Math.abs(parseFloat(style.width)*scale-bounds.width)<1,'scrollbar stays at the full panel edge');
 bounds={width:640,height:240};observer();assert.equal(instance.rows,30);assert.equal(instance.cols,100);
 assert.equal(instance.options.disableStdin,true);
 console.log('Mirror layout: PASS (remote grid preserved, bounded scaling, full-width scrollbar, resize, read only)');
})().catch(error=>{console.error(error);process.exitCode=1});
