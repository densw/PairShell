// The customer observes the existing PTY; it never opens a second shell.
var PairShellMirror = (() => {
  let terminal, cursor = 0, state = '', timer, polling = false, stopped = false, terminalRunning = false, endPolls = 0;
  const status = () => document.querySelector('#mirror-status');
  let remoteCols=80, remoteRows=24;
  function fitMirror(fromRemote = false) {
    const host=document.querySelector('#customer-terminal');
    const rect=host.getBoundingClientRect?.();
    if (!rect || rect.width<20 || rect.height<20) { if(fromRemote)terminal.resize(remoteCols,remoteRows); return; }
    // Preserve the PTY grid so full-screen applications keep their cursor positions.
    terminal.resize(remoteCols,remoteRows);
    const screen=terminal.element?.querySelector('.xterm-screen');
    if (!screen?.clientWidth || !screen.clientHeight) return;
    const scale=Math.min(1,(rect.width-8)/screen.clientWidth,(rect.height-2)/screen.clientHeight);
    terminal.element.style.width=(rect.width/scale)+'px';
    terminal.element.style.height=screen.clientHeight+'px';
    terminal.element.style.transformOrigin='top left';
    terminal.element.style.transform='scale('+scale+')';
  }
  function resize(width, height) {
    if (Number.isInteger(width) && width >= 2 && width <= 1000 && Number.isInteger(height) && height >= 1 && height <= 1000) {remoteCols=width;remoteRows=height;fitMirror(true);}
  }
  function write(data) {
    const bytes = Uint8Array.from(atob(data), char => char.charCodeAt(0));
    return new Promise(resolve => terminal.write(bytes, resolve));
  }
  async function poll() {
    if (polling || stopped) return;
    polling = true;
    try {
      const response = await controlFetch('/api/terminal?after=' + cursor);
      if (!response.ok) throw Error('view unavailable');
      const view = await response.json();
      if (view.truncated) {
        cursor = 0;
        terminal.reset();
        resize(view.width, view.height);
        document.querySelector('#mirror-note').textContent = '当前显示最近的终端内容；较早画面已移出实时缓冲，可在操作记录中查看。';
      }
      for (const event of view.events) {
        if (event.sequence <= cursor) continue;
        if (event.type === 'start') {
          terminalRunning = true;
          terminal.reset(); resize(event.width, event.height);
          document.querySelector('#mirror-note').textContent = '与协助方同一终端画面，按窗口缩放；仅供观看和复制，不能输入命令。';
        } else if (event.type === 'resize') resize(event.width, event.height);
        else if (event.type === 'output' && event.data) { terminalRunning = true; await write(event.data); }
        else if (event.type === 'end') { terminalRunning = false; if (state !== 'ENDED') status().textContent = '此终端已关闭 · 等待协助方重新打开'; }
        cursor = event.sequence;
      }
      cursor = Math.max(cursor, view.cursor || 0);
      if (state === 'ACTIVE' && view.events.some(event => event.type === 'output') && !view.events.some(event => event.type === 'end')) status().textContent = '实时同步 · 只读';
      if (state === 'ENDED') {
        endPolls++;
        if ((!terminalRunning && view.events.length === 0) || endPolls >= 40) stopped = true;
        status().textContent = stopped ? '协助已结束 · 画面已保留' : '协助已结束 · 正在同步最后画面';
      }
    } catch (_) {
      status().textContent = state === 'ENDED' ? '协助已结束 · 画面已保留' : '画面同步中断 · 正在重试';
      if (state === 'ENDED') {
        stopped = true;
        document.querySelector('#mirror-note').textContent = '协助已结束，保留已同步画面；最后一段输出可能尚未同步，完整内容以本机操作记录为准。';
      }
    } finally {
      polling = false;
      if (!stopped) timer = setTimeout(poll, 150);
    }
  }
  return {
    setState(next) {
      if (!terminal && ['ACTIVE', 'PAUSED', 'ENDED'].includes(next)) {
        terminal = new Terminal({disableStdin:true,cursorBlink:false,fontSize:13,lineHeight:1.2,cols:80,rows:24,scrollback:5000,fontFamily:'"Cascadia Mono", "SFMono-Regular", Consolas, monospace',theme:{background:'#10151d',foreground:'#d7e2f1'}});
        terminal.open(document.querySelector('#customer-terminal'));
        if(terminal.textarea){terminal.textarea.readOnly=true;terminal.textarea.setAttribute('aria-label','只读终端，可选择和复制输出');}
        if(typeof ResizeObserver!=='undefined')new ResizeObserver(()=>fitMirror()).observe(document.querySelector('#customer-terminal'));
        timer = setTimeout(poll, 0);
      }
      const changed = state !== next;
      state = next;
      document.querySelector('#terminal-mirror').hidden = !terminal;
      if(terminal)fitMirror();
      if (next === 'PAUSED') status().textContent = '已暂停新输入 · 已运行程序的输出仍同步';
      else if (next === 'ENDED') { status().textContent = stopped ? '协助已结束 · 画面已保留' : '协助已结束 · 正在同步最后画面'; if(changed && !polling){clearTimeout(timer);timer=setTimeout(poll,0);} }
      else if (next === 'ACTIVE' && terminal) status().textContent = terminalRunning ? '实时同步 · 只读' : (cursor ? '此终端已关闭 · 等待协助方重新打开' : '等待协助方打开终端 · 只读');
    }
  };
})();
