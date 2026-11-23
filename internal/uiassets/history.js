/* Local history annotations are editable; original audit contents are read only. */
(() => {
 const token=new URLSearchParams(location.hash.slice(1)).get('token')||sessionStorage.getItem('pairshell-window-token');
 const open=document.querySelector('#show-history');if(!open||!token)return;
 const dialog=document.createElement('dialog');dialog.className='session-dialog history-dialog';
 dialog.innerHTML='<div class="dialog-heading"><h2>历史操作记录</h2><button type="button" class="icon-close" data-history="close" aria-label="关闭历史记录">×</button></div><div class="history-tools"><input data-history="search" placeholder="搜索标题、备注或会话编号" aria-label="搜索历史"><button type="button" data-history="search-go">搜索</button><button type="button" data-history="new">新建备注</button><label><input type="checkbox" data-history="trash">回收站</label></div><p data-history="message" role="status"></p><div class="history-layout"><div data-history="list" class="history-list" aria-label="历史列表"></div><section class="history-detail"><label>标题<input data-history="title" maxlength="100"></label><label>备注<textarea data-history="note" maxlength="8192"></textarea></label><p data-history="info" class="caption"></p><div class="history-actions"><button type="button" data-history="save">保存备注</button><button type="button" data-history="delete">移至回收站</button></div><pre data-history="events" class="history-events" tabindex="0" aria-label="操作记录，只读"></pre><p data-history="loading" class="caption"></p></section></div><p class="caption">修改仅影响标题和备注；删除可在回收站恢复，原始审计内容不会被改写。</p>';
 document.body.appendChild(dialog);
 const el=id=>dialog.querySelector('[data-history="'+id+'"]');
 let selected=null,cursor=0,loading=false,done=false,generation=0;
 async function request(path,body){const r=await fetch('/history'+path,{method:body===undefined?'GET':'POST',headers:{'X-PairShell':token,'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});if(!r.ok)throw Error(await r.text());return r.json()}
 function error(e){el('message').textContent=e.message}
 async function loadList(){
  const list=await request('?q='+encodeURIComponent(el('search').value)+'&trash='+(el('trash').checked?'1':'0'));
  el('list').replaceChildren();
  if(!list.length)el('list').textContent='暂无记录';
  for(const entry of list){const b=document.createElement('button');b.type='button';b.className='history-entry';b.textContent=entry.title+'\n'+new Date(entry.created).toLocaleString()+(entry.kind==='note'?' · 手动备注':entry.complete?' · 已结束':' · 未完整结束');b.onclick=()=>select(entry);el('list').appendChild(b)}
 }
 async function more(){
  if(!dialog.open||loading||done||!selected||selected.kind==='note')return;
  const own=generation,id=selected.id;let received=false;loading=true;el('loading').textContent='正在读取…';
  try{
   const events=await request('/events?id='+encodeURIComponent(id)+'&after='+cursor);
   if(own!==generation)return;
   for(const event of events){cursor=Math.max(cursor,event.sequence);el('events').textContent+=event.sequence+' '+event.type+'\n'+event.data+'\n\n'}
   received=true;done=events.length===0;el('loading').textContent=done?'已显示全部记录':'向下滚动继续读取';
  }catch(e){if(own===generation){error(e);el('loading').textContent='读取失败，滚动可重试'}}
  finally{if(own===generation){loading=false;if(received&&!done&&el('events').scrollHeight<=el('events').clientHeight)await more()}}
 }
 async function select(entry){
  generation++;selected=entry;cursor=0;done=false;loading=false;
  el('title').value=entry.title;el('note').value=entry.note;el('events').textContent=entry.kind==='note'?'手动备注，没有执行日志。':'';el('events').scrollTop=0;
  el('info').textContent=entry.id;el('delete').textContent=entry.deleted?'恢复记录':'移至回收站';
  await more();
 }
 el('events').onscroll=()=>{const e=el('events');if(e.scrollTop+e.clientHeight>=e.scrollHeight-100)more()};
 dialog.onclose=()=>{generation++;loading=false};
 el('close').onclick=()=>dialog.close();
 open.onclick=async()=>{dialog.showModal();el('message').textContent='';try{await loadList()}catch(e){error(e)}};
 el('search-go').onclick=()=>loadList().catch(error);el('search').onkeydown=e=>{if(e.key==='Enter'){e.preventDefault();loadList().catch(error)}};
 el('trash').onchange=()=>loadList().catch(error);
 el('new').onclick=async()=>{try{const v=await request('/create',{Title:'手动备注',Note:''});el('trash').checked=false;await loadList();await select(v)}catch(e){error(e)}};
 el('save').onclick=async()=>{if(!selected)return;try{selected=await request('/update',{ID:selected.id,Title:el('title').value,Note:el('note').value});await loadList();el('message').textContent='标题与备注已保存'}catch(e){error(e)}};
 el('delete').onclick=async()=>{if(!selected)return;try{await request('/delete',{ID:selected.id,Deleted:!selected.deleted});selected=null;generation++;el('events').textContent='';el('title').value='';el('note').value='';await loadList();el('message').textContent='已更新，可在回收站查看和恢复'}catch(e){error(e)}};
})();
