const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const nodes=new Map(),slots=Array.from({length:3},()=>({dataset:{},textContent:''}));
const el=id=>{if(!nodes.has(id))nodes.set(id,{value:'',textContent:'',dataset:{},disabled:false,querySelectorAll:()=>slots,focus(){}});return nodes.get(id)};
let joins=0;
class Terminal{loadAddon(){}open(){}writeln(){}onData(){}onResize(){}attachCustomKeyEventHandler(){}}
const ctx=vm.createContext({URLSearchParams,TextEncoder,Terminal,FitAddon:{FitAddon:class{fit(){}}},ResizeObserver:class{observe(){}},document:{querySelector:el},history:{replaceState(){}},location:{hash:'#token=fixture'},sessionStorage:{getItem(){},setItem(){}},navigator:{clipboard:{}},setInterval(){},fetch:async path=>{if(path==='/merchant/join')joins++;return {ok:true,json:async()=>path.endsWith('/state')?{Phase:'idle',CanConnect:true}: {}}}});
vm.runInContext(fs.readFileSync('internal/uiassets/merchant.js','utf8'),ctx);
(async()=>{
 for(let i=0;i<40;i++)await Promise.resolve();
 el('#pair-code').value='１２３４-５６７８-９０１２';el('#pair-code').oninput();
 assert.equal(el('#pair-code').value,'123456789012');assert.equal(slots.map(s=>s.textContent).join(''),'123456789012');
 el('#pair-code').value='\u200e２８３７–８８１７–９００９\u200f';el('#pair-code').oninput();assert.equal(el('#pair-code').value,'283788179009');assert.equal(el('#pair-code').ariaInvalid,'false');
 el('#pair-code').value='12345678901234';el('#pair-code').oninput();await el('#connect-form').onsubmit({preventDefault(){}});assert.equal(joins,0,'legacy 14-digit codes are not supported');
 el('#pair-code').value='123';await el('#connect-form').onsubmit({preventDefault(){}});assert.equal(joins,0);
 el('#pair-code').value='123456123456789';el('#pair-code').oninput();assert.equal(el('#pair-code').ariaInvalid,'true');await el('#connect-form').onsubmit({preventDefault(){}});assert.equal(joins,0,'long paste must never be silently truncated and submitted');
 el('#pair-code').value='1234-5678-9012';await el('#connect-form').onsubmit({preventDefault(){}});assert.equal(joins,1);
 console.log('Pair code entry: PASS (12 digits, 3 groups, full-width paste, invalid lengths, complete submit)');
})().catch(error=>{console.error(error);process.exitCode=1});
