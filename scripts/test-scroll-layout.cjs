// Uses the existing browser runtime; no product dependency is required.
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {chromium}=require(process.env.PAIRSHELL_PLAYWRIGHT||'playwright');
const root=path.resolve(__dirname,'../internal/uiassets');
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.PAIRSHELL_CHROMIUM});
 try{
  const page=await browser.newPage();
  await page.route('http://pairshell.test/**',route=>{
   const name=new URL(route.request().url()).pathname.slice(1)||'merchant.html';
   const file=path.join(root,name);
   let body=fs.readFileSync(file);
   if(name.endsWith('.html'))body=body.toString().replace(/<script[\s\S]*?<\/script>/g,'');
   return route.fulfill({body,contentType:name.endsWith('.css')?'text/css':'text/html'});
  });
  for(const width of [640,850,1298,1587]){
   await page.setViewportSize({width,height:550});
   await page.goto('http://pairshell.test/merchant.html');
   await page.evaluate(()=>{
    document.querySelector('#verify-section').hidden=false;
    document.querySelector('#peer-code').textContent='4d18f04d34daccfed8654ff7';
   });
   const code=await page.locator('#peer-code').evaluate(e=>{
    const r=document.createRange();r.selectNodeContents(e);
    return {lines:r.getClientRects().length,fits:e.scrollWidth<=e.clientWidth,whiteSpace:getComputedStyle(e).whiteSpace};
   });
   assert.equal(code.lines,1,`verification wraps at ${width}`);assert.ok(code.fits,`verification clipped at ${width}`);
   const rail=await page.locator('.connection-pane').evaluate(e=>{
    e.append(Object.assign(document.createElement('div'),{style:'height:1500px'}));e.scrollTop=100;
    return {scroll:e.scrollTop,bar:getComputedStyle(e).scrollbarWidth};
   });
   assert.ok(rail.scroll>0);assert.equal(rail.bar,'none');
  }
  for(const width of [320,640,850,1587]){
   await page.setViewportSize({width,height:550});
   await page.goto('http://pairshell.test/control.html');
   const code=await page.locator('#verification').evaluate(e=>{
    document.body.dataset.session='AWAITING_CONSENT';
    e.textContent='4d18f04d34daccfed8654ff7';
    const r=document.createRange();r.selectNodeContents(e);
    return {lines:r.getClientRects().length,fits:e.scrollWidth<=e.clientWidth};
   });
   assert.equal(code.lines,1,`customer verification wraps at ${width}`);assert.ok(code.fits,`customer verification clipped at ${width}`);
  }
  await page.setViewportSize({width:1298,height:650});
  await page.goto('http://pairshell.test/merchant.html');
  await page.evaluate(()=>sessionStorage.setItem('pairshell-window-token','test'));
  await page.addScriptTag({content:fs.readFileSync(path.join(root,'history.js'),'utf8')});
  await page.evaluate(()=>{
   const d=document.querySelector('.history-dialog');d.showModal();
   d.querySelector('[data-history=placeholder]').hidden=true;d.querySelector('[data-history=detail]').hidden=false;
   d.querySelector('.history-events').textContent='connection.started\n'.repeat(150);
   d.querySelector('.history-list').innerHTML='<button class="history-entry">发起协助 · 已结束</button>'.repeat(30);
   d.querySelector('textarea').value='备注';
   d.querySelector('[data-history=info]').textContent='session-merchant-'+ 'a'.repeat(64);
   d.querySelector('[data-history=loading]').textContent='已显示全部记录';
  });
  for(const selector of ['.history-list','.history-events']){
   const v=await page.locator(selector).evaluate(e=>{e.scrollTop=80;return {scroll:e.scrollTop,bar:getComputedStyle(e).scrollbarWidth}});
   assert.equal(v.bar,'none',selector);assert.ok(v.scroll>0,selector+' still scrolls');
  }
  for(const viewport of [{width:1298,height:996},{width:850,height:550},{width:640,height:550},{width:320,height:550}]){
   await page.setViewportSize(viewport);
   const dialog=page.locator('.history-dialog');
   await dialog.evaluate(e=>{e.scrollTop=0});
   const heading=await page.locator('.history-dialog .dialog-heading').boundingBox();
   // Wheel over the outer shell must never move the title or close button.
   await page.mouse.move(heading.x+20,heading.y+10);
   await page.mouse.wheel(0,800);
   await page.waitForTimeout(100);
   const layout=await dialog.evaluate(e=>{
    e.scrollTop=200;
    const rect=e.getBoundingClientRect(),heading=e.querySelector('.dialog-heading').getBoundingClientRect();
    const footer=e.lastElementChild.getBoundingClientRect(),detail=e.querySelector('.history-detail');
    return {scroll:e.scrollTop,overflow:e.scrollHeight-e.clientHeight,width:e.scrollWidth-e.clientWidth,
     heading:heading.top,footer:footer.bottom,bottom:rect.bottom,top:rect.top,
     detailScrollable:detail.scrollHeight>detail.clientHeight};
   });
   if(process.env.PAIRSHELL_SCREENSHOT)await page.screenshot({path:process.env.PAIRSHELL_SCREENSHOT});
   assert.equal(layout.scroll,0,`history shell must not scroll at ${viewport.width}`);
   assert.ok(layout.overflow<=1,`history contents must fit the shell at ${viewport.width}`);
   assert.ok(layout.width<=1,`history must not overflow horizontally at ${viewport.width}`);
   assert.equal(layout.heading,heading.y,'title and close button must remain fixed');
   assert.ok(layout.footer<layout.bottom&&layout.top>=0,'footer must stay inside the visible dialog');
   for(const selector of ['.history-list','.history-events']){
    const v=await page.locator(selector).evaluate(e=>{e.scrollTop=80;return {scroll:e.scrollTop,height:e.clientHeight}});
    assert.ok(v.height>0&&v.scroll>0,selector+' remains usable');
   }
   const detail=await page.locator('.history-detail').evaluate(e=>{
    e.scrollTop=e.scrollHeight;
    return {bottom:e.querySelector('[data-history=loading]').getBoundingClientRect().bottom,
     edge:e.getBoundingClientRect().bottom};
   });
   assert.ok(detail.bottom<=detail.edge+1,'detail status must remain reachable on short screens');
  }
  await page.setViewportSize({width:1298,height:996});
  if(process.env.PAIRSHELL_SCREENSHOT)await page.screenshot({path:process.env.PAIRSHELL_SCREENSHOT});
  console.log('Scroll layout: PASS (4 widths, fixed history shell, independent list/log scrolling, single-line verification, hidden bars)');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
