/* A window can host successive sessions; each socket remains bound to one attempt. */
let key = new URLSearchParams(location.hash.slice(1)).get('token') || sessionStorage.getItem('pairshell-window-token');
history.replaceState(null, '', '/merchant');
if (key) sessionStorage.setItem('pairshell-window-token', key);
const $ = selector => document.querySelector(selector);
$('#customer-mode').href = '/#token=' + encodeURIComponent(key || '');
const term = new Terminal({cursorBlink:true,fontSize:14,fontFamily:'"Cascadia Mono", "SFMono-Regular", Consolas, monospace',scrollback:5000,theme:{background:'#10151d',foreground:'#d7e2f1',cursor:'#a7bfff'}});
const fit = new FitAddon.FitAddon(); term.loadAddon(fit); term.open($('#terminal')); fit.fit();
term.writeln('PairShell 远程终端\r\n等待连接和客户授权…');
let terminalReady = false;
let socket = null, currentAttempt = null, terminalStarted = false, phase = 'idle', canConnect = true, canEnd = false;
let paneOpen = true, focusedAttempt = null;
let actionPending = false;
let needsVerification = false;
const encoder = new TextEncoder();
function send(type, extra) { if (terminalReady && socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify({type,...extra})); }
function closeTerminal() { terminalReady=false; const old = socket; socket = null; if (old) old.close(); }
function setConnectionPane(open) {
  if (!open) closeTools(false);
  paneOpen = open;
  $('.workspace').dataset.pane = open ? 'open' : 'closed';
  $('.connection-pane').hidden = !open;
  $('#toggle-connection').textContent = open ? '返回终端' : '连接与工具';
  $('#toggle-connection').ariaExpanded = String(open);
  try { fit.fit(); } catch (_) {}
  send('resize', {cols:term.cols,rows:term.rows});
}
function renderJourney() {
  const stage=phase==='active'?(terminalReady?'active':socket?'opening':'terminal_closed'):phase==='awaiting_consent'?(needsVerification?'verification':'consent'):phase;
  $('.workspace').dataset.stage=stage;
  if(document.body)document.body.dataset.connectionStage=stage;
  if(stage==='opening'){ $('#merchant-state').textContent='正在打开终端';$('#merchant-state').dataset.phase='opening'; }
  if(stage==='active'){ $('#merchant-state').textContent='已连接';$('#merchant-state').dataset.phase='active'; }
  const titles={idle:'连接设备，开始协助',pairing:'正在建立配对',verification:'请先核对双方校验码',consent:'等待客户允许本次协助',opening:'授权已完成，正在打开终端',terminal_closed:'本次终端已关闭',active:'终端已就绪',ending:'正在结束本次连接',ended:'本次连接已结束',error:'这次连接没有完成'};
  $('#journey-title').textContent=titles[stage]||'正在准备连接';
  $('#terminal-empty').ariaHidden=String(stage==='active');
  $('#terminal').ariaHidden=String(!['active','terminal_closed'].includes(stage));
  if(term.textarea)term.textarea.tabIndex=terminalReady?0:-1;
  $('#return-connect').hidden=!['ended','error'].includes(stage);
  $('#new-connection').hidden=phase!=='active';
  $('#reopen-terminal').hidden=stage!=='terminal_closed';
  const step=stage==='verification'?0:['pairing','consent','opening','active'].indexOf(stage);
  for(const [index,id] of ['step-pair','step-consent','step-terminal'].entries()){
    $('#'+id).dataset.state=stage==='active'?'done':step>=0?(index<step?'done':index===step?'current':'waiting'):'waiting';
  }
  if(stage==='opening')$('#session-hint').textContent='正在准备远端 Shell。终端就绪后会自动进入，你现在仍可取消。';
  if(stage==='verification')$('#session-hint').textContent='通过已有的聊天或电话联系客户，在侧栏输入对方设备上的校验码。';
  if(stage==='terminal_closed')$('#session-hint').textContent='终端已关闭。连接可用时可重新打开终端，或结束并重新配对。';
}
function focusConnection(clear) {
  if(clear)$('#pair-code').value='';
  setConnectionPane(true);renderCodeSlots();$('#pair-code').focus?.();
}
function renderCodeSlots() {
  const input=$('#pair-code'), digits=input.value.replace(/[^0-9]/g,'');
  const slots=$('#code-slots').querySelectorAll?.('span') || [];
  const cursor=input.value.slice(0,input.selectionStart ?? input.value.length).replace(/[^0-9]/g,'').length;
  slots.forEach((slot,index)=>{slot.textContent=digits.slice(index*4,index===2?undefined:index*4+4)||'••••';slot.dataset.current=String(index===Math.min(Math.floor(cursor/4),2));});
}
$('#pair-code').oninput=()=>{
  const input=$('#pair-code');
  const value=input.value.replace(/[０-９]/g,char=>String.fromCharCode(char.charCodeAt(0)-65248)).replace(/[-\s]/g,'');
  input.value=value;
  const invalid=/[^0-9]/.test(value)||value.length>14;
  input.ariaInvalid=String(invalid);$('#merchant-error').textContent=invalid?'请输入 12 位配对码；也支持旧版 14 位码':'';
  renderCodeSlots();
};
$('#pair-code').onkeyup=renderCodeSlots;
$('#pair-code').onclick=event=>{
  const input=$('#pair-code');
  if(event && !event.shiftKey && input.selectionStart===input.selectionEnd){
    const slots=[...($('#code-slots').querySelectorAll?.('span')||[])];
    const index=slots.findIndex(slot=>{const r=slot.getBoundingClientRect();return event.clientY>=r.top&&event.clientY<=r.bottom&&event.clientX<r.right;});
    if(index>=0){const r=slots[index].getBoundingClientRect();const count=index===2&&input.value.length>12?6:4;const offset=Math.min(count,Math.max(0,Math.round((event.clientX-r.left)/r.width*count)));const position=Math.min(index*4+offset,input.value.length);input.setSelectionRange?.(position,position);}
  }
  renderCodeSlots();
};
function serverLabel(address) { return address ? '已配置自定义服务' : '未配置自定义服务'; }
const toolIDs = ['invite-options','settings','adapter-options'];
function closeTools(restoreFocus = true) {
  const current = toolIDs.find(id => $('#'+id).open);
  for (const id of toolIDs) $('#'+id).open = false;
  $('#tool-backdrop').hidden = true;
  if (restoreFocus && current) $('#'+current).querySelector?.('summary')?.focus();
}
for (const id of toolIDs) {
  const panel = $('#'+id);
  panel.ontoggle = () => {
    if (panel.open) {
      for (const other of toolIDs) if (other !== id) $('#'+other).open = false;
      panel.setAttribute?.('role', 'dialog');
      panel.setAttribute?.('aria-modal', 'true');
      panel.setAttribute?.('aria-label', {settings:'服务器配置','invite-options':'邀请连接','adapter-options':'CLI / MCP'}[id]);
      panel.querySelector?.('input:not([disabled]), button')?.focus();
    } else {
      panel.removeAttribute?.('role');
      panel.removeAttribute?.('aria-modal');
    }
    $('#tool-backdrop').hidden = !toolIDs.some(tool => $('#'+tool).open);
  };
  $('#close-'+id).onclick = () => closeTools();
}
$('#tool-backdrop').onclick = () => closeTools();
document.addEventListener?.('keydown', event => {
  const current = toolIDs.find(id => $('#'+id).open);
  if (!current) return;
  if (event.key === 'Escape') { event.preventDefault(); closeTools(); }
  if (event.key === 'Tab') {
    const controls = [...$('#'+current).querySelectorAll('summary, input:not([disabled]), button:not([disabled]), textarea, a[href]')];
    const first = controls[0], last = controls[controls.length-1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
  }
});
function renderRoute(route) {
  const el = $('#route-status');
  el.hidden = phase !== 'active';
  const kind = {direct:'直连',relay:'中继'}[route.selected];
  const rtt = Number(route.rtt_ms);
  const measured = kind && Number.isFinite(rtt) && rtt > 0;
  const quality = !measured ? 'unknown' : rtt < 80 ? 'good' : rtt <= 200 ? 'fair' : 'poor';
  const label = {good:'流畅',fair:'稍慢',poor:'高延迟',unknown:'测量中'}[quality];
  el.dataset.quality = quality;
  el.textContent = kind ? kind + (measured ? ' · '+rtt.toFixed(1)+' ms · '+label : ' · 测量中') : '线路测量中';
  el.title = '当前线路与往返延迟：'+el.textContent;
}
term.onData(data => { if (!terminalReady || !socket || socket.readyState !== WebSocket.OPEN) return; let chunk=''; for(const char of data){if(encoder.encode(chunk+char).length>12000){send('input',{data:chunk});chunk=''}chunk+=char}if(chunk)send('input',{data:chunk}); });
term.onResize(({cols,rows}) => send('resize',{cols,rows}));
term.attachCustomKeyEventHandler(event => { if(event.type!=='keydown')return true;const shortcut=event.metaKey||(event.ctrlKey&&event.shiftKey);if(shortcut&&event.code==='KeyC'){const selection=term.getSelection();if(selection&&navigator.clipboard)navigator.clipboard.writeText(selection).catch(()=>{});return false}if(shortcut&&event.code==='KeyV'){if(navigator.clipboard)navigator.clipboard.readText().then(text=>term.paste(text)).catch(()=>{});return false}return true; });
new ResizeObserver(()=>{try{fit.fit()}catch(_){}}).observe($('#terminal'));
function openTerminal() {
  if (terminalStarted || phase !== 'active') return;
  terminalStarted = true; terminalReady=false; $('#reopen-terminal').hidden = true;
  const attempt = currentAttempt;
  const ws = new WebSocket((location.protocol==='https:'?'wss:':'ws:')+'//'+location.host+'/merchant/terminal');
  socket=ws; ws.binaryType='arraybuffer';
  const owns = () => socket === ws && currentAttempt === attempt;
  ws.onopen=()=>{if(!owns()){ws.close();return}fit.fit();ws.send(JSON.stringify({token:key,attempt_id:attempt,cols:Math.max(2,term.cols),rows:Math.max(1,term.rows)}));term.reset();$('#terminal-status').textContent='正在打开终端…';renderJourney();};
  ws.onmessage=event=>{if(!owns())return;if(event.data instanceof ArrayBuffer){term.write(new Uint8Array(event.data));return}try{const status=JSON.parse(event.data);if(status.ready&&Number(status.attempt_id)===attempt){terminalReady=true;$('#terminal-status').textContent='交互终端 · 已连接';if(focusedAttempt!==attempt){focusedAttempt=attempt;setConnectionPane(false)}renderJourney();term.focus();send('resize',{cols:term.cols,rows:term.rows});}if(status.error){setConnectionPane(true);$('#merchant-error').textContent=status.error;term.writeln('\r\n[终端未开启：'+status.error+']')}}catch(_){}};
  ws.onclose=()=>{if(!owns())return;socket=null;terminalReady=false;$('#terminal-status').textContent='终端已关闭';$('#reopen-terminal').hidden=phase!=='active';renderJourney();term.writeln('\r\n[终端已关闭。需要继续时，请手动重新打开终端。]');};
  ws.onerror=()=>{if(owns())$('#merchant-error').textContent='终端连接中断。请检查对方连接状态；不会自动重放输入。';};
}
async function call(path,body){const r=await fetch('/merchant/'+path,{method:'POST',headers:{'X-PairShell':key,'Content-Type':'application/json'},body:JSON.stringify(body||{})});if(!r.ok)throw Error(await r.text());return r.json();}
const phaseLabel={idle:'等待连接',pairing:'正在配对',awaiting_consent:'等待对方授权',active:'已连接',ending:'正在结束',ended:'连接已结束',error:'连接未完成'};
const verificationInput = $('#merchant-verification-input');
function refreshConfirmButton() {
  $('#confirm').disabled = !needsVerification || actionPending || !/^[a-f0-9]{24}$/i.test((verificationInput.value || '').trim());
}
async function refresh(){
  const r=await fetch('/merchant/state',{headers:{'X-PairShell':key}});if(!r.ok)throw Error('窗口认证失效，请重新打开客户端');const s=await r.json();
  const attempt=Number(s.AttemptID||0);if(currentAttempt!==null&&attempt<currentAttempt)return;
  if(currentAttempt!==attempt){closeTerminal();currentAttempt=attempt;terminalStarted=false;focusedAttempt=null;needsVerification=false;verificationInput.value='';setConnectionPane(true);$('#reopen-terminal').hidden=true;term.reset();term.writeln('PairShell 远程终端\r\n等待连接和客户授权…');}
  const previousPhase=phase;
  phase=s.Phase||({已连接:'active',已结束:'ended'})[s.State]||(s.State?'pairing':'idle');
  if(phase==='ended'&&previousPhase!=='ended')$('#pair-code').value='';
  $('.workspace').dataset.phase=phase;
  $('#session-hint').textContent=({idle:'输入左侧配对码。客户允许后，这里就是远程设备的终端。',pairing:'正在验证配对信息，请稍候。你可以随时取消。',awaiting_consent:'等待客户核对并允许本次协助。',ended:'本次连接已结束。输入新的配对码即可再次连接。',error:'连接未完成，请根据左侧提示检查配对码或服务器后重试。'})[phase]||'正在准备终端…';
  canConnect=s.CanConnect===undefined?['idle','error','ended'].includes(phase):s.CanConnect;
  canEnd=s.CanEnd===undefined?['pairing','awaiting_consent','active'].includes(phase):s.CanEnd;
  $('#role-switch').hidden=canEnd;
  $('#merchant-state').textContent=phaseLabel[phase]||'连接状态更新中';
  $('#merchant-state').dataset.phase=phase;
  $('#toggle-connection').hidden=phase!=='active';
  if(phase!=='active'&&!paneOpen)setConnectionPane(true);
  renderCodeSlots();$('#connect').disabled=!canConnect||actionPending;$('#pair-code').disabled=!canConnect||actionPending;$('#create').disabled=!canConnect||actionPending;
  $('#connect').textContent=actionPending?'正在处理…':canConnect?(attempt?'重新连接':'连接'):(phase==='active'?'已连接':phase==='awaiting_consent'?'等待对方授权…':'连接中…');
  $('#end').disabled=!canEnd;$('#end').textContent=phase==='active'?'结束连接':'取消连接';
  $('#end').hidden=!canEnd;
  for(const id of ['server','credential','save-settings'])$('#'+id).disabled=!canConnect||actionPending;
  $('#invitation').value=s.Invite||'';$('#remote-system').textContent=s.System||'目标系统待确认';
  $('#adapter').textContent=s.Reference?'pairshell mcp --session "'+s.Reference+'"':'连接后自动生成本机会话引用';
  const verifyNow=s.State==='等待商家核对身份',showVerification=verifyNow&&!needsVerification;needsVerification=verifyNow;
  $('#peer-code').textContent=s.Code||'';
  $('#verify-section').hidden=phase!=='awaiting_consent'||!s.Code;
  $('#verify-form').hidden=!verifyNow;
  refreshConfirmButton();
  if(showVerification){setConnectionPane(true);$('#invite-options').open=false;$('#settings').open=false;$('#verify-section').scrollIntoView?.({block:'nearest'});verificationInput.focus?.();}
  renderRoute(s.Route||{});
  if(phase==='error')$('#merchant-error').textContent=s.Error||s.State||'配对未完成。请核对配对码后重试。';
  if(phase==='active')openTerminal();else{closeTerminal();$('#reopen-terminal').hidden=true;$('#terminal-status').textContent=phase==='ended'?'终端已关闭':'连接后自动打开终端';}
  renderJourney();
}
async function action(fn){if(actionPending)return;actionPending=true;$('#connect').disabled=true;$('#create').disabled=true;$('#connect').textContent='正在处理…';try{$('#merchant-error').textContent='';await fn()}catch(e){$('#merchant-error').textContent=e.message}finally{actionPending=false;await refresh().catch(()=>{});}}
$('#connect-form').onsubmit=event=>{event.preventDefault();if(!canConnect||actionPending)return;return action(async()=>{const code=$('#pair-code').value.trim();const valid=/^(?:\d{12}|\d{14})$/.test(code.replace(/[-\s]/g,''));$('#pair-code').ariaInvalid=String(!valid);if(!valid){$('#pair-code').focus?.();throw Error('请输入完整的 12 位配对码')}await call('join',{Code:code});});};
$('#create').onclick=()=>{if(canConnect)action(async()=>{await call('create',{Token:$('#credential').value});$('#credential').value='';});};
verificationInput.oninput=refreshConfirmButton;
$('#verify-form').onsubmit=event=>{
  event.preventDefault();
  if ($('#confirm').disabled) return;
  const entered = verificationInput.value.trim();
  return action(async ()=>{
    refreshConfirmButton();
    await call('confirm', {attempt_id:currentAttempt, verification_code:entered});
    verificationInput.value='';
  });
};
$('#end').onclick=()=>{if(canEnd)action(async()=>{closeTerminal();await call('end');});};
$('#new-connection').onclick=async()=>{if(actionPending)return;await action(async()=>{closeTerminal();await call('end');});if(canConnect)focusConnection(true);};
$('#return-connect').onclick=()=>focusConnection(phase==='ended');
$('#toggle-connection').onclick=()=>{if(phase==='active'){setConnectionPane(!paneOpen);if(!paneOpen&&terminalReady)term.focus();}};
$('#reopen-terminal').onclick=()=>{if(phase==='active'){terminalStarted=false;openTerminal();}};
$('#save-settings').onclick=()=>{if(canConnect)action(async()=>{const address=$('#server').value.trim();try{await call('settings',{Server:address,force_custom:$('#force-custom').checked});$('#server-summary').textContent=serverLabel(address);$('#settings-feedback').textContent='已保存，下次连接生效';closeTools();$('#settings-notice').textContent='服务器配置已保存';}catch(error){$('#settings').open=true;$('#settings-feedback').textContent=error.message;throw error;}});};
fetch('/merchant/settings',{headers:{'X-PairShell':key}}).then(r=>r.ok?r.json():null).then(s=>{if(s){$('#server').value=s.Server||s.server||'';$('#force-custom').checked=!!s.force_custom;$('#official-status').textContent=s.official_available?(s.official_server?'官方通道：'+s.official_server+'。自定义服务优先。':'自定义服务优先；不强制时可由官方服务兜底。'):'请配置自定义服务器。';$('#server-summary').textContent=serverLabel($('#server').value)}}).catch(()=>{});
refresh().catch(e=>$('#merchant-error').textContent=e.message);setInterval(()=>refresh().catch(e=>$('#merchant-error').textContent=e.message),1000);
