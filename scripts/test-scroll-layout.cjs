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
  });
  for(const selector of ['.history-dialog','.history-list','.history-events']){
   const v=await page.locator(selector).evaluate(e=>{e.scrollTop=80;return {scroll:e.scrollTop,bar:getComputedStyle(e).scrollbarWidth}});
   assert.equal(v.bar,'none',selector);assert.ok(v.scroll>0,selector+' still scrolls');
  }
  if(process.env.PAIRSHELL_SCREENSHOT)await page.screenshot({path:process.env.PAIRSHELL_SCREENSHOT});
  console.log('Scroll layout: PASS (4 widths, full single-line verification, history and rail retain scrolling without bars)');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
