import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const source = readFileSync(new URL('./static/asset-delete-reveal.js', import.meta.url), 'utf8');
test('the deletion footer stays in document flow before the pull starts',()=>{
  assert.equal(page().footer.hidden,false);
});
function page() {
  let now = 0, modal = false, reduced = false, timerID = 0; const timers = new Map();
  const listeners = {}, root = {scrollTop:0, clientHeight:600, scrollHeight:2000,calls:[]};
  const target = {closest:()=>null, parentElement:root};
  const link = {focus(){this.focused=true;}};
  const footer = {hidden:true, inert:false, offsetHeight:100, getBoundingClientRect:()=>({top:1800-root.scrollTop}), calls:[], scrollIntoView(options){this.calls.push(options);}, querySelector:()=>link};
  root.scrollTo = options => {root.calls.push(options);if(options.top===root.scrollHeight-root.clientHeight)footer.calls.push(options);root.scrollTop=options.top;};
  vm.runInNewContext(source, {
    document:{scrollingElement:root,body:root,querySelector:selector=>selector==='[data-delete-reveal]' ? footer : modal},
    window:{addEventListener:(type,fn)=>listeners[type]=fn}, performance:{now:()=>now},
    matchMedia:()=>({matches:reduced}),getComputedStyle:()=>({overflowY:'auto'}),
    setTimeout:(fn,ms)=>{const id=++timerID;timers.set(id,{fn,at:now+ms});return id;},clearTimeout:id=>timers.delete(id)
  });
  return {root,footer,link, bottom(){root.scrollTop=1200;listeners.scroll();}, wait(ms){const end=now+ms;for(const [id,timer] of timers){if(timer.at<=end){timers.delete(id);now=timer.at;timer.fn();}}now=end;},modal(value){modal=value;},reduced(){reduced=true;},
    send(type,values={}){listeners[type]({target,deltaX:0,deltaY:0,preventDefault(){},...values});}};
}
test('arriving at bottom and the rest of the same wheel gesture keep deletion hidden',()=>{
  const p=page();p.send('wheel',{deltaY:1200});p.bottom();
  for(let n=0;n<10;n++){p.wait(80);p.send('wheel',{deltaY:20});}
  assert.equal(p.footer.inert,true);
  p.wait(300);p.send('wheel',{deltaY:240});assert.equal(p.footer.inert,false);assert.equal(p.footer.calls.length,1);
  p.send('wheel',{deltaY:50});assert.equal(p.footer.calls.length,1);
});
test('upward, horizontal, zoom and nested scroll gestures do not reveal',()=>{
  for(const values of [{deltaY:-240},{deltaX:1000,deltaY:240},{deltaY:240,ctrlKey:true},{deltaY:240,target:{closest:()=>null,scrollHeight:200,clientHeight:100}}]){
    const p=page();p.bottom();p.wait(300);p.send('wheel',values);assert.equal(p.footer.inert,true);
  }
  const p=page();p.bottom();p.wait(300);p.modal(true);p.send('wheel',{deltaY:240});assert.equal(p.footer.inert,true);
});
test('a wheel event delivered just after reaching the bottom still belongs to the arrival',()=>{
  const p=page();p.bottom();p.send('wheel',{deltaY:100});assert.equal(p.footer.inert,true);
  p.wait(300);p.send('wheel',{deltaY:240});assert.equal(p.footer.inert,false);
});
test('touch requires a fresh upward swipe starting at the bottom',()=>{
  const p=page();p.send('touchstart',{touches:[{clientY:300}]});p.bottom();p.send('touchmove',{touches:[{clientY:100}]});assert.equal(p.footer.inert,true);
  p.send('touchend');p.send('touchstart',{touches:[{clientY:300}]});p.send('touchmove',{touches:[{clientY:280}]});assert.equal(p.footer.inert,true);
  p.send('touchmove',{touches:[{clientY:240}]});assert.equal(p.footer.inert,true);
  p.send('touchmove',{touches:[{clientY:200}]});assert.equal(p.footer.inert,false);
  p.send('touchmove',{touches:[{clientY:220}]});assert.equal(p.footer.inert,true);
  p.send('touchend');p.send('touchstart',{touches:[{clientY:300}]});p.send('touchmove',{touches:[{clientY:200}]});assert.equal(p.footer.inert,false);
});
test('keyboard repeat cannot expose it on first arrival; a fresh key reveals and focuses it',()=>{
  const p=page();p.send('keydown',{key:'End'});p.bottom();p.send('keydown',{key:'End',repeat:true});assert.equal(p.footer.inert,true);
  p.reduced();p.send('keydown',{key:'ArrowDown'});assert.equal(p.footer.inert,false);assert.equal(p.link.focused,true);assert.equal(p.footer.calls[0].behavior,'instant');
  let prevented=false;p.send('keydown',{key:'ArrowDown',preventDefault(){prevented=true;}});assert.equal(prevented,false);
});
test('Tab at bottom reveals the link without moving focus out of normal tab order',()=>{
  const p=page();p.bottom();p.send('keydown',{key:'Tab'});assert.equal(p.footer.inert,false);assert.equal(p.link.focused,undefined);
});
test('markup provides full-width action, initial hiding and a no-JavaScript fallback',()=>{
  const html=readFileSync(new URL('./templates/asset.html',import.meta.url),'utf8');
  const css=readFileSync(new URL('./static/app.css',import.meta.url),'utf8');
  assert.match(html,/data-delete-reveal hidden/);assert.match(html,/<noscript><div class="asset-delete-reveal">/);
  assert.match(css,/\.asset-delete-reveal > \.button[^}]*width:100%/);
});
test('bottom anchor resists a light wheel pull and closes again on upward scrolling',()=>{
  const p=page();p.bottom();p.wait(300);
  p.send('wheel',{deltaY:60});assert.equal(p.footer.inert,true);
  p.wait(50);p.send('wheel',{deltaY:180});assert.equal(p.footer.inert,false);
  p.send('wheel',{deltaY:-10});assert.equal(p.footer.inert,true);
  p.wait(300);p.send('wheel',{deltaY:60});assert.equal(p.footer.inert,true);
  p.wait(50);p.send('wheel',{deltaY:180});assert.equal(p.footer.inert,false);
});
test('passive scroll adjustments preserve the latch; keyboard return releases it and resets the pull',()=>{
  const p=page();p.bottom();p.wait(300);p.send('wheel',{deltaY:100});p.wait(300);p.send('wheel',{deltaY:140});assert.equal(p.footer.inert,true);
  p.wait(50);p.send('wheel',{deltaY:100});assert.equal(p.footer.inert,false);
  p.root.scrollTop=1180;p.send('scroll');assert.equal(p.footer.inert,false);
  p.send('keydown',{key:'Home'});assert.equal(p.footer.inert,true);
  p.bottom();p.wait(300);p.send('wheel',{deltaY:100});assert.equal(p.footer.inert,true);
});
test('reveal remains latched through smooth scrolling, layout adjustment and downward momentum',()=>{
  for(const reduced of [false,true]){
    const p=page();if(reduced)p.reduced();p.bottom();p.wait(300);p.send('wheel',{deltaY:240});
    p.root.scrollHeight=1900;
    for(const top of [1220,1250,1230,1300,1295,1300]){
      p.root.scrollTop=top;p.send('scroll');p.wait(30);p.send('wheel',{deltaY:20});assert.equal(p.footer.inert,false);
    }
    p.wait(2000);p.send('scroll');assert.equal(p.footer.inert,false);assert.equal(p.footer.calls.length,1);
    p.send('wheel',{deltaY:-10});assert.equal(p.footer.inert,true);
    p.send('wheel',{deltaY:500});assert.equal(p.footer.inert,true);
    p.wait(300);p.send('wheel',{deltaY:60});assert.equal(p.footer.inert,true);
    p.wait(50);p.send('wheel',{deltaY:180});assert.equal(p.footer.inert,false);
  }
});
test('line and page wheel units also cross the anchor',()=>{
  for(const values of [{deltaY:15,deltaMode:1},{deltaY:1,deltaMode:2}]){
    const p=page();p.bottom();p.wait(300);p.send('wheel',values);assert.equal(p.footer.inert,false);
  }
});
test('reveal uses the same document-end stop and consumes downward wheel input while latched',()=>{
  for(const deltaY of [240,1200]){
    const p=page();p.bottom();p.wait(300);let prevented=false;
    p.send('wheel',{deltaY,preventDefault(){prevented=true;}});
    assert.equal(prevented,true);
    assert.equal(p.footer.calls[0].top,p.root.scrollHeight-p.root.clientHeight);
    assert.equal(p.footer.calls[0].behavior,'smooth');
    prevented=false;p.send('wheel',{deltaY:50,preventDefault(){prevented=true;}});
    assert.equal(prevented,true);assert.equal(p.footer.calls.length,1);
    prevented=false;p.send('wheel',{deltaY:-10,preventDefault(){prevented=true;}});
    assert.equal(prevented,true);assert.equal(p.footer.inert,true);
  }
});
test('delete footer has a clear separator and consistent spacing',()=>{
  const css=readFileSync(new URL('./static/app.css',import.meta.url),'utf8');
  assert.match(css,/\.asset-delete-reveal\s*\{[^}]*border-top:1px solid var\(--muted\)/);
  assert.match(css,/\.asset-delete-reveal\s*\{[^}]*padding:24px 0/);
});
test('touch reveal consumes further downward motion but allows reversal',()=>{
  const p=page();p.bottom();p.send('touchstart',{touches:[{clientY:300}]});
  for(const clientY of [200,180]){
    let prevented=false;p.send('touchmove',{touches:[{clientY}],preventDefault(){prevented=true;}});
    assert.equal(prevented,true);assert.equal(p.footer.inert,false);
  }
  assert.equal(p.footer.calls.length,1);
  let prevented=false;p.send('touchmove',{touches:[{clientY:200}],preventDefault(){prevented=true;}});
  assert.equal(prevented,true);assert.equal(p.footer.inert,true);
});
test('wheel pressure progressively uncovers real footer content and an incomplete pull rebounds',()=>{
  const p=page();p.bottom();p.wait(300);
  p.send('wheel',{deltaY:60});const hint=p.root.scrollTop;
  assert.ok(hint>1200 && hint<1300);assert.equal(p.footer.hidden,false);assert.equal(p.footer.inert,true);
  p.wait(50);p.send('wheel',{deltaY:80});assert.ok(p.root.scrollTop>hint && p.root.scrollTop<1300);
  p.wait(230);assert.equal(p.root.scrollTop,1200);assert.equal(p.root.calls.at(-1).behavior,'smooth');
  assert.equal(p.footer.hidden,false);assert.equal(p.footer.inert,true);
});
test('a small touch drag follows the finger, cancels smoothly, and remains unclickable',()=>{
  for(const finish of ['touchend','touchcancel']){
    const p=page();p.bottom();p.send('touchstart',{touches:[{clientY:300}]});
    p.send('touchmove',{touches:[{clientY:276}]});const hint=p.root.scrollTop;
    assert.ok(hint>1200 && hint<1300);assert.equal(p.footer.inert,true);
    p.send('touchmove',{touches:[{clientY:250}]});assert.ok(p.root.scrollTop>hint);
    p.send(finish);assert.equal(p.root.scrollTop,1200);assert.equal(p.footer.hidden,false);
  }
});
test('native scroll and resize cannot skip the closed anchor',()=>{
  const p=page();p.root.scrollTop=1400;p.send('scroll');assert.equal(p.root.scrollTop,1200);
  p.root.clientHeight=700;p.send('resize');assert.equal(p.root.scrollTop,1100);assert.equal(p.footer.inert,true);
});
