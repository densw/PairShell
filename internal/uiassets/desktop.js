const startupParams=new URLSearchParams(location.hash.slice(1));
let token=startupParams.get('token')||sessionStorage.getItem('pairshell-window-token');
const startNew=startupParams.get('new')==='1';
if(token)sessionStorage.setItem('pairshell-window-token',token);
history.replaceState(null,'','/');
const $=selector=>document.querySelector(selector);
$('#merchant-mode').href='/merchant#token='+encodeURIComponent(token||'');
let displayedCode='',redirected=false,currentAttempt=null,canGenerate=false,canReset=false,refreshing=false;
let handoffGeneration=0;
async function handoff(controlURL,attempt){
 const generation=++handoffGeneration;redirected=true;
 $('#pairing-handoff').hidden=false;$('#handoff-open').href=controlURL;
 $('#copy-code').disabled=true;$('#regenerate-code').disabled=true;$('#start').disabled=true;
 if(typeof requestAnimationFrame==='function')await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
 if(generation===handoffGeneration&&currentAttempt===attempt)location.assign(controlURL);
}
async function api(path,body){const r=await fetch(path,{method:body===undefined?'GET':'POST',headers:{'X-PairShell':token,'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});if(!r.ok)throw Error(await r.text());return r.json();}
async function status(){
 const v=await api('/status');const attempt=Number(v.AttemptID||0);if(currentAttempt!==null&&attempt<currentAttempt)return v;
 if(currentAttempt!==attempt){currentAttempt=attempt;redirected=false;++handoffGeneration;$('#pairing-handoff').hidden=true;$('#copy-code').textContent='复制配对码';}
 displayedCode=v.PairCode||'';canGenerate=v.CanGenerate===undefined?!v.Control:v.CanGenerate;canReset=!!v.CanReset;
 $('#pairing-workspace').dataset.phase=v.Phase||'waiting';
 $('#state').textContent=v.State||'等待连接';$('#pair-code').textContent=displayedCode||((v.Phase==='error'||v.Phase==='ended')?'暂无有效配对码':'正在准备…');
 $('#copy-code').disabled=!displayedCode;$('#regenerate-code').disabled=refreshing||(!canGenerate&&!canReset);
 $('#regenerate-code').textContent=refreshing?'正在刷新…':canGenerate?(displayedCode?'刷新配对码':'重新生成配对码'):(canReset?'取消并重新配对':'正在建立连接…');
 $('#start').disabled=!canGenerate;$('#code').textContent=v.Code||'';
 const control=$('#control');control.hidden=!v.Control;
 if(v.Control&&!redirected){control.href=v.Control;await handoff(v.Control,attempt);}
 return v;
}
async function waitForIdle(){for(let count=0;count<100;count++){const v=await status();if(v.CanGenerate)return;await new Promise(resolve=>setTimeout(resolve,150));}throw Error('上次协助仍在结束，请稍后重试');}
async function generate(reset){if(refreshing)return;refreshing=true;$('#regenerate-code').disabled=true;try{if(reset){await api('/reset',{});await waitForIdle();}await api('/listen',{});await status();}finally{refreshing=false;$('#regenerate-code').disabled=false;}}
$('#copy-code').onclick=async()=>{try{if(!displayedCode)return;await navigator.clipboard.writeText(displayedCode);$('#copy-code').textContent='已复制';}catch(_){$('#state').textContent='请选中配对码后复制';}};
$('#regenerate-code').onclick=async()=>{try{await generate(!canGenerate&&canReset);}catch(e){$('#state').textContent=e.message;}};
$('#save-server').onclick=async()=>{const button=$('#save-server');if(button.disabled)return;button.disabled=true;try{await api('/settings',{server:$('#server').value.trim(),force_custom:$('#force-custom').checked});$('#settings-error').textContent='已保存，正在准备新的配对码…';await generate(canReset);$('#settings-error').textContent='服务器已保存，新配对码将使用此服务器';}catch(e){$('#settings-error').textContent=e.message;}finally{button.disabled=false;}};
$('#start').onclick=async()=>{if(!canGenerate)return;try{await api('/start',{invite:$('#invite').value.trim()});await status();}catch(e){$('#state').textContent=e.message;}};
$('#handoff-back').onclick=async()=>{++handoffGeneration;try{await generate(true);redirected=false;$('#pairing-handoff').hidden=true;}catch(e){$('#handoff-error').textContent=e.message;}};
(async()=>{try{const prefs=await api('/settings');$('#server').value=prefs.server||'';$('#force-custom').checked=!!prefs.force_custom;$('#official-status').textContent=prefs.official_available?(prefs.official_server?'官方通道：'+prefs.official_server+'。自定义配置优先。':'留空时使用官方服务；自定义配置优先。'):'请配置自定义服务器。';const initial=await status();if(startNew){await generate(true);}else if(initial.CanGenerate!==false&&!initial.PairCode&&!initial.Control){await generate(false);}}catch(e){$('#state').textContent=e.message;$('#regenerate-code').disabled=false;}})();
setInterval(()=>status().catch(e=>$('#state').textContent=e.message),1000);
