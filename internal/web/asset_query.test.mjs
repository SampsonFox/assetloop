import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

function harness({missingCount=false}={}) {
  function el(extra={}) { return Object.assign({hidden:false, listeners:{},attrs:{},addEventListener(k,f){this.listeners[k]=f;},setAttribute(k,v){this.attrs[k]=v;},removeAttribute(k){delete this.attrs[k];},getAttribute(k){return this.attrs[k]||'';},contains(x){return x===this;},focus(){doc.activeElement=this;},classList:{toggle(){},contains(){return false;}},querySelector(){},querySelectorAll(){return []; }},extra); }
  const inputs=Object.fromEntries(['q','status','sort','direction','view'].map(k=>[k,el({value:({sort:'created',direction:'desc',view:'grid'})[k]||'',type:'text',checked:false})]));
  const summary=el({childNodes:[],replaceChildren(){}}), menu=el({open:false,querySelector:()=>summary}), clear=el({href:'/assets/a',hidden:true}), retry=el(), login=el(), status=el();
  const feedback=el({querySelector:s=>s==='button'?retry:s==='a'?login:status});
  const form=el({elements:{namedItem:k=>inputs[k]},querySelector:s=>s==='details'?menu:s==='summary'?summary:clear});
  let replacements=0;
  let results=el({replaceWith(next){replacements++;results=next;}});
  const count=el({textContent:'1'});
  const root=el({dataset:{loading:'loading',updated:'updated',session:'session',failed:'failed'},querySelector:s=>s==='.asset-filters'?form:s==='[data-asset-feedback]'?feedback:s==='[data-asset-count]'?count:results});
  const doc=el({activeElement:inputs.q,querySelector:()=>root,dispatchEvent(){}});
  const location=new URL('http://localhost/?unrelated=keep');
  const timers=new Map(), requests=[]; let timerID=0;
  const context={document:doc,location,URL,URLSearchParams,AbortController,Event,
    FormData:class {constructor(){return Object.entries(inputs).filter(([,v])=>v.type!=='checkbox'||v.checked).map(([k,v])=>[k,v.type==='checkbox'?'1':v.value]);}},
    DOMParser:class {parseFromString(){return {querySelector:s=>s==='[data-asset-results]'?el({replaceWith(next){replacements++;results=next;}}):s==='[data-asset-count]'?(missingCount?null:el({textContent:'2'})):form};}},
    history:{replaceState(_,__,url){location.href=new URL(url,location).href;}},
    setTimeout(fn,ms){timers.set(++timerID,{fn,ms});return timerID;},clearTimeout(id){timers.delete(id);},
    fetch(url,options){return new Promise((resolve,reject)=>requests.push({url:new URL(url),options,resolve,reject}));}};
  context.window={fetch:context.fetch,AbortController,scrollX:0,scrollY:100,scrollTo(){}};
  vm.runInNewContext(readFileSync(new URL('./static/asset-query.js',import.meta.url),'utf8'),context);
  return {inputs,menu,form,root,doc,status,retry,login,requests,location,count,feedback,get replacements(){return replacements;},
    tick(ms){for(const [id,t] of [...timers]) if(t.ms===ms){timers.delete(id);t.fn();}},
    submit(apply=false){form.listeners.submit({preventDefault(){},submitter:{hasAttribute:()=>apply}});},
    async finish(i=0,code=200,destination=requests[i].url.href){requests[i].resolve({ok:code===200,status:code,url:destination,text:async()=>''});await new Promise(setImmediate);}};
}
test('search debounces and never includes draft advanced filters',()=>{
  const h=harness();h.inputs.status.value='repair';h.inputs.q.value='abc';
  h.inputs.q.listeners.input();assert.equal(h.requests.length,0);h.tick(300);
  assert.equal(h.requests.length,1);assert.equal(h.requests[0].url.searchParams.get('status'),null);
  assert.equal(h.requests[0].url.searchParams.get('q'),'abc');assert.equal(h.requests[0].url.searchParams.get('unrelated'),'keep');
});
test('composition waits; Enter submits immediately without default navigation',()=>{
  const h=harness();h.inputs.q.listeners.compositionstart();h.inputs.q.listeners.input();h.tick(300);assert.equal(h.requests.length,0);
  h.inputs.q.listeners.compositionend();h.inputs.q.listeners.keydown({key:'Enter',preventDefault(){}});h.tick(300);assert.equal(h.requests.length,1);
});
test('advanced options submit together, close only on success, and synchronize applied filters',async()=>{
  const h=harness();h.menu.open=true;h.inputs.direction.value='asc';h.inputs.status.value='repair';
  assert.equal(h.requests.length,0);h.submit(true);assert.equal(h.requests[0].url.searchParams.get('direction'),'asc');
  assert.equal(h.menu.open,true);await h.finish();assert.equal(h.menu.open,false);
  h.inputs.status.value='sale';h.submit();assert.equal(h.requests[1].url.searchParams.get('status'),'repair');
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

test('updates results and count while preserving toolbar, input focus and silent feedback',async()=>{
  const h=harness(), input=h.inputs.q, form=h.form;
  h.inputs.q.value='phone';h.submit();await h.finish();
  assert.equal(h.replacements,1);assert.equal(h.count.textContent,'2');
  assert.equal(h.inputs.q,input);assert.equal(h.form,form);assert.equal(h.doc.activeElement,input);
  assert.equal(h.feedback.hidden,true);assert.equal(h.location.searchParams.get('view'),'grid');
});
test('sort, page, view and clear links refresh locally; modified clicks stay native',async()=>{
  for(const query of ['?page=2&view=grid','?sort=name&direction=asc','?view=list','?sort=created&direction=desc']){
    const h=harness();let prevented=false;
    const link={href:'http://localhost/'+query};
    h.root.listeners.click({target:{closest:()=>link},button:0,ctrlKey:true,preventDefault(){prevented=true;}});
    assert.equal(h.requests.length,0);assert.equal(prevented,false);
    h.root.listeners.click({target:{closest:()=>link},button:0,preventDefault(){prevented=true;}});
    assert.equal(prevented,true);await h.finish();assert.equal(h.replacements,1);
    assert.equal(h.location.search,query);
  }
});
test('failed responses leave results untouched',async()=>{
  const h=harness();h.submit();await h.finish(0,500);
  assert.equal(h.replacements,0);assert.equal(h.feedback.hidden,false);
});
test('incomplete HTML does not partially replace results',async()=>{
  const h=harness({missingCount:true});h.submit();await h.finish();
  assert.equal(h.replacements,0);assert.equal(h.status.textContent,'failed');
});
test('redirected login is not inserted and canonical page redirects update the URL',async()=>{
  const h=harness();h.submit();await h.finish(0,200,'http://localhost/login');
  assert.equal(h.replacements,0);assert.equal(h.login.hidden,false);
  h.submit();await h.finish(1,200,'http://localhost/?view=grid&page=1');
  assert.equal(h.replacements,1);assert.equal(h.location.searchParams.get('page'),'1');
});
test('server template retains native GET fallback and stable enhancement boundaries',()=>{
  const html=readFileSync(new URL('./templates/assets.html',import.meta.url),'utf8');
  for(const marker of ['data-asset-results tabindex="-1"','method="get"','data-asset-count','data-asset-apply','data-asset-clear','/static/asset-query.js']){
    assert.ok(html.includes(marker),marker);
  }
});
