const controlTokenKey='pairshell.control';
let controlToken='',eventCursor=0,sessionEnded=false,returnURL='',busy=false,timer,lostConnection=false;
const verificationInput=document.querySelector('#customer-verification-input');
let customerState='LOADING';
function refreshConsentButton(){
 const button=document.querySelector('button[data-action="consent"]');
 button.disabled=busy||sessionEnded||customerState!=='AWAITING_CONSENT'||!/^[a-f0-9]{24}$/i.test((verificationInput.value||'').trim());
}
verificationInput.oninput=refreshConsentButton;
function controlFetch(path, options={}) {
 return fetch(path,{...options,credentials:'omit',headers:{...options.headers,'X-PairShell-Control':controlToken}});
}
function renderReturn(){const a=document.querySelector('#new-session');if(a){a.hidden=!returnURL||(!sessionEnded&&!lostConnection);if(returnURL)a.href=returnURL}document.querySelector('#session-finish').hidden=!sessionEnded;const back=document.querySelector('#back-pairing');back.hidden=!returnURL||sessionEnded;back.disabled=busy;back.textContent=lostConnection?'返回配对并结束旧会话':'结束并返回配对';}
function rememberReturn(value){if(!value)return;try{const u=new URL(value);if(u.protocol==='http:'&&u.hostname==='127.0.0.1'&&u.port&&u.pathname==='/'&&!u.username&&!u.password){const fragment=new URLSearchParams(u.hash.slice(1));fragment.set('new','1');u.hash=fragment.toString();returnURL=u.href;history.replaceState({pairingReturn:returnURL},'','/')}}catch(_){}}
function renderEnded(){sessionEnded=true;document.body.dataset.session='ENDED';PairShellMirror.setState('ENDED');document.querySelector('#session-state').textContent='会话已结束';document.querySelector('#session-state').dataset.state='ENDED';document.querySelectorAll('button[data-action]').forEach(button=>{button.disabled=true;button.hidden=true});renderReturn();clearInterval(timer);}
async function call(p,b){const r=await controlFetch('/api/'+p,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(b||{})});if(!r.ok)throw Error(await r.text());return r.json()}
function connectionError(e){const text=document.querySelector('#error');lostConnection=true;if(document.body.dataset.session==='LOADING')document.body.dataset.session='UNAVAILABLE';text.textContent=sessionEnded?'本次协助已结束，可以开始新的协助。':(e instanceof TypeError?'本机会话连接不可用。返回配对时会先结束旧会话。':e.message);renderReturn()}
async function refresh(){
 const r=await controlFetch('/api/session');if(!r.ok)throw Error('本机会话认证失败');const v=await r.json();rememberReturn(v.return_url);sessionEnded=v.session.state==='ENDED';
 lostConnection=false;renderReturn();
 customerState=v.session.state;document.body.dataset.session=customerState;PairShellMirror.setState(customerState);
 document.querySelector('#verification').textContent=v.verification_code||'尚未完成身份校验';document.querySelector('#session-state').textContent=({ACTIVE:'协助进行中',PAUSED:'已暂停',AWAITING_CONSENT:'等待你的授权',ENDED:'会话已结束'})[v.session.state]||v.session.state;document.querySelector('#session-state').dataset.state=v.session.state;
 document.querySelectorAll('button[data-action]').forEach(b=>{b.hidden=({consent:customerState!=='AWAITING_CONSENT',pause:customerState!=='ACTIVE',resume:customerState!=='PAUSED',end:customerState==='ENDED'})[b.dataset.action];b.disabled=busy||b.hidden});
 refreshConsentButton();
 document.querySelector('#status').textContent=JSON.stringify({verification_code:v.verification_code,file_root:v.file_root,...v.session},null,2);
 if(sessionEnded){renderEnded();return}
 const tr=await controlFetch('/api/tasks');if(tr.ok){const tasks=await tr.json();document.querySelector('#outputs').textContent=tasks.map(t=>t.id+' ['+t.state+'] bytes '+t.output_start+'..'+t.output_end+'\n'+t.output).join('\n\n')}
 const er=await controlFetch('/api/events?after='+eventCursor);if(er.ok){for(const ev of await er.json()){eventCursor=Math.max(eventCursor,ev.sequence);const el=document.querySelector('#events');el.textContent+=ev.sequence+' '+ev.type+' '+ev.data+'\n';if(el.textContent.length>262144)el.textContent=el.textContent.slice(-262144)}}
}
async function start(){
 const params=new URLSearchParams(location.hash.slice(1)),token=params.get('bootstrap');
 rememberReturn(history.state?.pairingReturn);rememberReturn(params.get('return'));
 history.replaceState({pairingReturn:returnURL},'','/');
 try{controlToken=sessionStorage.getItem(controlTokenKey)||''}catch(_){}
 if(token){
  const r=await fetch('/api/bootstrap',{method:'POST',credentials:'omit',headers:{'X-Bootstrap':token}});
  if(r.ok){controlToken=(await r.json()).token;try{sessionStorage.setItem(controlTokenKey,controlToken)}catch(_){}}
  else if(!controlToken)throw Error('此协助页面已失效，请返回配对重新开始');
 }
 await refresh();if(sessionEnded)return;
 timer=setInterval(async()=>{if(busy||sessionEnded)return;try{await call('heartbeat');await refresh()}catch(e){connectionError(e)}},2000);
}
document.querySelectorAll('button[data-action]').forEach(b=>b.onclick=async()=>{
 if(busy||b.disabled)return;
 const action=b.dataset.action;
 busy=true;b.disabled=true;
 const error=document.querySelector('#error');error.textContent='';
 try{
  let body;
  if(action==='consent'){
   const caps={};['terminal','tasks','file_read','file_write'].forEach(k=>caps[k]=document.getElementById(k).checked);
   body={capabilities:caps,ai:document.getElementById('ai').checked,verification_code:verificationInput.value.trim()};
  }
  const result=await call(action,body);
  if(action==='consent')verificationInput.value='';
  if(result.state==='ENDED'){renderEnded()}else{busy=false;await refresh()}
 }catch(e){
  if(action==='consent'&&!(e instanceof TypeError))error.textContent=e.message;else connectionError(e)
 }finally{busy=false;if(!sessionEnded&&!b.hidden)b.disabled=false;refreshConsentButton();renderReturn()}
});
document.querySelector('#back-pairing').onclick=async()=>{if(busy||!returnURL)return;busy=true;renderReturn();document.querySelector('#back-pairing').textContent='正在结束并返回…';try{if(!sessionEnded&&!lostConnection){await call('end');renderEnded()}location.assign(returnURL)}catch(error){connectionError(error)}finally{busy=false;renderReturn()}};
let recordKind='status';
function renderRecords(){
 const text=document.querySelector('#'+recordKind).textContent || '暂无记录';
 document.querySelector('#record-page').textContent=text;
 for(const kind of ['status','outputs','events'])document.querySelector('#record-'+kind).ariaPressed=String(kind===recordKind);
}
for(const kind of ['status','outputs','events'])document.querySelector('#record-'+kind).onclick=()=>{recordKind=kind;renderRecords();document.querySelector('#record-page').scrollTop=0};
document.querySelector('#show-records').onclick=()=>{renderRecords();document.querySelector('#records-dialog').showModal()};
document.querySelector('#close-records').onclick=()=>document.querySelector('#records-dialog').close();
start().catch(connectionError);
