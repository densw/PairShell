const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const nodes=new Map();const el=id=>{if(!nodes.has(id))nodes.set(id,{dataset:{},textContent:'',showModal(){},close(){}});return nodes.get(id)};
const ctx=vm.createContext({URL,URLSearchParams,document:{body:{dataset:{}},querySelector:el,querySelectorAll:()=>[]},PairShellMirror:{setState(){}},location:{hash:''},history:{replaceState(){}},setInterval(){},clearInterval(){},fetch:async()=>({ok:true,json:async()=>({csrf:'fixture',session:{state:'ENDED'}})})});
vm.runInContext(fs.readFileSync('internal/uiassets/control.js','utf8'),ctx);
(async()=>{
 for(let i=0;i<50;i++)await Promise.resolve();
 const original='路径与输出'.repeat(150)+'END';el('#status').textContent=original;el('#show-records').onclick();
 assert.equal(el('#record-page').textContent,original,'scroll view must retain every output character');
 const html=fs.readFileSync('internal/uiassets/control.html','utf8');assert.ok(!html.includes('record-next')&&!html.includes('record-prev'),'no manual paging controls');
 const css=fs.readFileSync('internal/uiassets/ui.css','utf8');assert.match(css,/\.session-dialog \.record-page\{[^}]*overflow:auto/);
 console.log('Record scroll: PASS (complete text, no paging, scrollable viewport)');
})().catch(error=>{console.error(error);process.exitCode=1});
