const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
async function scenario(authorized){
 const nodes=new Map(),navigations=[];
 const el=id=>{if(!nodes.has(id))nodes.set(id,{dataset:{},textContent:'',hidden:false,disabled:true});return nodes.get(id)};
 const back='http://127.0.0.1:2222/#token=fixture';
 const context=vm.createContext({URL,URLSearchParams,document:{body:{dataset:{session:'LOADING'}},querySelector:el,querySelectorAll:()=>[],getElementById:el},PairShellMirror:{setState(){}},location:{hash:'#bootstrap=used&return='+encodeURIComponent(back),assign:url=>navigations.push(url)},history:{replaceState(){}},setInterval(){},clearInterval(){},sessionStorage:{getItem:()=>authorized?'fixture':''},fetch:async (path,options)=>({ok:path!=='/api/bootstrap'&&(path!=='/api/session'||options.headers['X-PairShell-Control']==='fixture'),json:async()=>path==='/api/session'?{csrf:'fixture',return_url:back,session:{state:'ACTIVE'}}:[]})});
 vm.runInContext(fs.readFileSync('internal/uiassets/control.js','utf8'),context);
 for(let i=0;i<60;i++)await Promise.resolve();
 assert.equal(el('#back-pairing').hidden,false,'a consumed bootstrap must retain a return entry');
 if(authorized)assert.equal(el('#session-state').textContent,'协助进行中','an existing tab-scoped header token may recover the session');
 else {
  assert.equal(el('#new-session').hidden,false);
  await el('#back-pairing').onclick();
  assert.match(navigations[0],/new=1/,'recovery must reset the old session');
 }
}
(async()=>{await scenario(false);await scenario(true);console.log('Session recovery: PASS (expired bootstrap, authenticated reload, return to new pairing)')})().catch(error=>{console.error(error);process.exitCode=1});
