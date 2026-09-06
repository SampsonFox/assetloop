import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

function harness() {
  function el(extra={}) { return Object.assign({hidden:false, listeners:{},attrs:{},addEventListener(k,f){this.listeners[k]=f;},setAttribute(k,v){this.attrs[k]=v;},removeAttribute(k){delete this.attrs[k];},getAttribute(k){return this.attrs[k]||'';},contains(x){return x===this;},focus(){doc.activeElement=this;},classList:{toggle(){},contains(){return false;}},querySelector(){},querySelectorAll(){return []; }},extra); }
  const inputs=Object.fromEntries(['q','event_type','sort','direction','show_voided'].map(k=>[k,el({value:({sort:'occurred',direction:'asc'})[k]||'',type:k==='show_voided'?'checkbox':'text',checked:false})]));
  const summary=el(), menu=el({open:false,querySelector:()=>summary}), clear=el({href:'/assets/a',hidden:true}), retry=el(), login=el(), status=el();
  const feedback=el({querySelector:s=>s==='button'?retry:s==='a'?login:status});
  const form=el({elements:{namedItem:k=>inputs[k]},querySelector:s=>s==='details'?menu:s==='summary'?summary:clear});
  let results=el({replaceWith(next){results=next;}});
  const root=el({dataset:{loading:'loading',updated:'updated',session:'session',failed:'failed'},querySelector:s=>s==='.timeline-filters'?form:s==='[data-timeline-feedback]'?feedback:results});
  const doc=el({activeElement:inputs.q,querySelector:()=>root,dispatchEvent(){}});
  const location=new URL('http://localhost/assets/a?unrelated=keep');
  const timers=new Map(), requests=[]; let timerID=0;
  const context={document:doc,location,URL,URLSearchParams,AbortController,Event,
    FormData:class {constructor(){return Object.entries(inputs).filter(([,v])=>v.type!=='checkbox'||v.checked).map(([k,v])=>[k,v.type==='checkbox'?'1':v.value]);}},
    DOMParser:class {parseFromString(){return {querySelector:s=>s==='[data-timeline-results]'?el({replaceWith(next){results=next;}}):form};}},
    history:{replaceState(_,__,url){location.href=new URL(url,location).href;}},
    setTimeout(fn,ms){timers.set(++timerID,{fn,ms});return timerID;},clearTimeout(id){timers.delete(id);},
    fetch(url,options){return new Promise((resolve,reject)=>requests.push({url:new URL(url),options,resolve,reject}));}};
  context.window={fetch:context.fetch,AbortController,scrollX:0,scrollY:100,scrollTo(){}};
  vm.runInNewContext(readFileSync(new URL('./static/timeline-query.js',import.meta.url),'utf8'),context);
  return {inputs,menu,form,root,doc,status,retry,login,requests,location,
    tick(ms){for(const [id,t] of [...timers]) if(t.ms===ms){timers.delete(id);t.fn();}},
    submit(apply=false){form.listeners.submit({preventDefault(){},submitter:{hasAttribute:()=>apply}});},
    async finish(i=0,code=200){requests[i].resolve({ok:code===200,status:code,url:requests[i].url.href,text:async()=>''});await new Promise(setImmediate);}};
}
test('search debounces and never includes draft advanced filters',()=>{
  const h=harness();h.inputs.event_type.value='repair';h.inputs.q.value='abc';
  h.inputs.q.listeners.input();assert.equal(h.requests.length,0);h.tick(300);
  assert.equal(h.requests.length,1);assert.equal(h.requests[0].url.searchParams.get('event_type'),null);
  assert.equal(h.requests[0].url.searchParams.get('q'),'abc');assert.equal(h.requests[0].url.searchParams.get('unrelated'),'keep');
});
test('composition waits; Enter submits immediately without default navigation',()=>{
  const h=harness();h.inputs.q.listeners.compositionstart();h.inputs.q.listeners.input();h.tick(300);assert.equal(h.requests.length,0);
  h.inputs.q.listeners.compositionend();h.inputs.q.listeners.keydown({key:'Enter',preventDefault(){}});h.tick(300);assert.equal(h.requests.length,1);
});
test('advanced options submit together, close only on success, and synchronize applied filters',async()=>{
  const h=harness();h.menu.open=true;h.inputs.show_voided.checked=true;h.inputs.event_type.value='repair';
  assert.equal(h.requests.length,0);h.submit(true);assert.equal(h.requests[0].url.searchParams.get('show_voided'),'1');
  assert.equal(h.menu.open,true);await h.finish();assert.equal(h.menu.open,false);
  h.inputs.event_type.value='sale';h.submit();assert.equal(h.requests[1].url.searchParams.get('event_type'),'repair');
});
test('new input invalidates in-flight responses before the debounce expires',async()=>{
  const h=harness();h.inputs.q.value='old';h.submit();h.inputs.q.value='new';h.inputs.q.listeners.input();
  assert.equal(h.requests[0].options.signal.aborted,true);await h.finish();assert.equal(h.location.searchParams.get('q'),null);
  h.tick(300);await h.finish(1);assert.equal(h.location.searchParams.get('q'),'new');
});
test('failure retains URL and offers retry; expired session offers login',async()=>{
  const h=harness();h.inputs.q.value='new';h.submit();await h.finish(0,500);
  assert.equal(h.status.textContent,'failed');assert.equal(h.retry.hidden,false);assert.equal(h.location.searchParams.get('q'),null);
  h.retry.listeners.click();await h.finish(1,401);assert.equal(h.login.hidden,false);assert.equal(h.retry.hidden,true);
});
test('outside click and Escape close filters, internal clicks do not query or close',()=>{
  const h=harness();h.menu.open=true;h.doc.listeners.click({target:h.menu});assert.equal(h.menu.open,true);
  h.doc.listeners.click({target:{}});assert.equal(h.menu.open,false);h.menu.open=true;
  h.doc.listeners.keydown({key:'Escape',preventDefault(){}});assert.equal(h.menu.open,false);assert.equal(h.requests.length,0);
});
test('server template retains native forms but removes mixed automatic submission',()=>{
  const html=readFileSync(new URL('./templates/asset.html',import.meta.url),'utf8');
  assert.match(html,/data-timeline-results tabindex="-1"/);assert.match(html,/method="get"/);
  const checkbox=html.split('\n').find(x=>x.includes('name="show_voided"'));
  assert.ok(!checkbox.includes('data-auto-submit'));
});
