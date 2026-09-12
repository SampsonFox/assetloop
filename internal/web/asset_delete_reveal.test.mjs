import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const source = readFileSync(new URL('./static/asset-delete-reveal.js', import.meta.url), 'utf8');
function page() {
  let now = 0, modal = false, reduced = false;
  const listeners = {}, root = {scrollTop:0, clientHeight:600, scrollHeight:1800};
  const target = {closest:()=>null, parentElement:root};
  const link = {focus(){this.focused=true;}};
  const footer = {hidden:true, calls:[], scrollIntoView(options){this.calls.push(options);}, querySelector:()=>link};
  vm.runInNewContext(source, {
    document:{scrollingElement:root,body:root,querySelector:selector=>selector==='[data-delete-reveal]' ? footer : modal},
    window:{addEventListener:(type,fn)=>listeners[type]=fn}, performance:{now:()=>now},
    matchMedia:()=>({matches:reduced}),getComputedStyle:()=>({overflowY:'auto'})
  });
  return {root,footer,link, bottom(){root.scrollTop=1200;listeners.scroll();}, wait(ms){now+=ms;},modal(value){modal=value;},reduced(){reduced=true;},
    send(type,values={}){listeners[type]({target,deltaX:0,deltaY:0,preventDefault(){},...values});}};
}
test('arriving at bottom and the rest of the same wheel gesture keep deletion hidden',()=>{
  const p=page();p.send('wheel',{deltaY:1200});p.bottom();
  for(let n=0;n<10;n++){p.wait(80);p.send('wheel',{deltaY:20});}
  assert.equal(p.footer.hidden,true);
  p.wait(300);p.send('wheel',{deltaY:50});assert.equal(p.footer.hidden,false);assert.equal(p.footer.calls.length,1);
  p.send('wheel',{deltaY:50});assert.equal(p.footer.calls.length,1);
});
test('upward, horizontal, zoom and nested scroll gestures do not reveal',()=>{
  for(const values of [{deltaY:-50},{deltaX:100,deltaY:1},{deltaY:50,ctrlKey:true},{deltaY:50,target:{closest:()=>null,scrollHeight:200,clientHeight:100}}]){
    const p=page();p.bottom();p.wait(300);p.send('wheel',values);assert.equal(p.footer.hidden,true);
  }
  const p=page();p.bottom();p.wait(300);p.modal(true);p.send('wheel',{deltaY:50});assert.equal(p.footer.hidden,true);
});
test('a wheel event delivered just after reaching the bottom still belongs to the arrival',()=>{
  const p=page();p.bottom();p.send('wheel',{deltaY:100});assert.equal(p.footer.hidden,true);
  p.wait(300);p.send('wheel',{deltaY:100});assert.equal(p.footer.hidden,false);
});
test('touch requires a fresh upward swipe starting at the bottom',()=>{
  const p=page();p.send('touchstart',{touches:[{clientY:300}]});p.bottom();p.send('touchmove',{touches:[{clientY:100}]});assert.equal(p.footer.hidden,true);
  p.send('touchend');p.send('touchstart',{touches:[{clientY:300}]});p.send('touchmove',{touches:[{clientY:280}]});assert.equal(p.footer.hidden,true);
  p.send('touchmove',{touches:[{clientY:240}]});assert.equal(p.footer.hidden,false);
});
test('keyboard repeat cannot expose it on first arrival; a fresh key reveals and focuses it',()=>{
  const p=page();p.send('keydown',{key:'End'});p.bottom();p.send('keydown',{key:'End',repeat:true});assert.equal(p.footer.hidden,true);
  p.reduced();p.send('keydown',{key:'ArrowDown'});assert.equal(p.footer.hidden,false);assert.equal(p.link.focused,true);assert.equal(p.footer.calls[0].behavior,'auto');
  let prevented=false;p.send('keydown',{key:'ArrowDown',preventDefault(){prevented=true;}});assert.equal(prevented,false);
});
test('Tab at bottom reveals the link without moving focus out of normal tab order',()=>{
  const p=page();p.bottom();p.send('keydown',{key:'Tab'});assert.equal(p.footer.hidden,false);assert.equal(p.link.focused,undefined);
});
test('markup provides full-width action, initial hiding and a no-JavaScript fallback',()=>{
  const html=readFileSync(new URL('./templates/asset.html',import.meta.url),'utf8');
  const css=readFileSync(new URL('./static/app.css',import.meta.url),'utf8');
  assert.match(html,/data-delete-reveal hidden/);assert.match(html,/<noscript><div class="asset-delete-reveal">/);
  assert.match(css,/\.asset-delete-reveal > \.button[^}]*width:100%/);
});
