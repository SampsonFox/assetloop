import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const source = readFileSync(new URL('./static/settings.js', import.meta.url), 'utf8');
function harness(initial = 'http://localhost/admin/catalog?q=phone', stored = '{}') {
  const el = (props = {}) => Object.assign({dataset:{},hidden:false,attrs:{},listeners:{},childNodes:[],
    addEventListener(k,f){this.listeners[k]=f;},querySelector(){return null;},querySelectorAll(){return [];},
    setAttribute(k,v){this.attrs[k]=v;},removeAttribute(k){delete this.attrs[k];},hasAttribute(k){return k in this.attrs;},contains(x){return x?.inside===this;},
    focus(){},replaceChildren(...nodes){this.childNodes=nodes;}},props);
  const requests=[], listeners={}, windows={}, writes=[], events=[];
  let current, dirty=false, submitting=false, allow=true, replaced=0, location=new URL(initial);
  const makeContent = () => el({querySelector(s){return s.includes('submitting') ? (submitting?{}:null) : s.includes('dirty') && dirty ? {dataset:{discardConfirm:'discard'}} : null;},
    replaceWith(next){current=next;replaced++;}});
  current=makeContent();
  const tabs=el(), retry=el(), login=el(), message=el();
  const feedback=el({dataset:{loading:'loading',failure:'failed',login:'expired',discard:'discard'},querySelector(s){return s==='span'?message:s.includes('retry')?retry:login;}});
  const doc={title:'catalog',getElementById:()=>current,querySelector(s){return s==='.settings-tabs'?tabs:s==='[data-settings-feedback]'?feedback:null;},
    addEventListener(k,f){listeners[k]=f;},dispatchEvent(e){events.push(e.type);}};
  const ctx={URL,URLSearchParams,AbortController,Event,console,document:doc,
    sessionStorage:{getItem:()=>stored,setItem(k,v){writes.push(v);}},
    setTimeout:()=>1,clearTimeout(){},history:{state:{},pushState(s,t,u){location=new URL(u,location);},replaceState(s,t,u){location=new URL(u,location);}},
    window:{confirm:()=>allow,addEventListener(k,f){windows[k]=f;}},
    DOMParser:class {parseFromString(){return {title:'loaded',getElementById:makeContent,querySelector:()=>el()};}},
    fetch(url,options){return new Promise((resolve,reject)=>requests.push({url,options,resolve,reject}));}};
  Object.defineProperty(ctx,'location',{get:()=>location});
  vm.runInNewContext(source,ctx);
  return {requests,feedback,retry,login,message,writes,events,tabs,doc,
    get current(){return current;},get replaced(){return replaced;},get location(){return location;},
    set dirty(v){dirty=v;},set submitting(v){submitting=v;},set allow(v){allow=v;},
    click(path,tab=true,ctrl=false){const link={href:new URL(path,location).href,inside:tab?tabs:current,hasAttribute:k=>k==='data-settings-tab'&&tab};
      let prevented=false;listeners.click({button:0,ctrlKey:ctrl,target:{closest:()=>link},preventDefault(){prevented=true;}});return prevented;},
    input(){listeners.input({target:{inside:current}});},
    pop(path){location=new URL(path,location);windows.popstate();},
    async finish(i=0,status=200,destination=requests[i].url){requests[i].resolve({ok:status===200,status,url:destination,text:async()=>''});await new Promise(setImmediate);}
  };
}
test('switch replaces only content, synchronizes history, and initializes new drawers',async()=>{
 const h=harness(), tabs=h.tabs, old=h.current;
 assert.ok(h.click('/admin/tags'));await h.finish();
 assert.notEqual(h.current,old);assert.equal(h.tabs,tabs);assert.equal(h.replaced,1);
 assert.equal(h.location.pathname,'/admin/tags');assert.deepEqual(h.events,['settings:loaded']);
 h.click('/admin/catalog');assert.equal(new URL(h.requests[1].url).search,'?q=phone');
});
test('newer navigation wins even if old server ignores cancellation',async()=>{
 const h=harness();h.click('/admin/tags');h.click('/admin/3d');
 assert.ok(h.requests[0].options.signal.aborted);await h.finish(1);await h.finish(0);
 assert.equal(h.replaced,1);assert.equal(h.location.pathname,'/admin/3d');
});
test('failure retains old content and URL, retry and login are explicit',async()=>{
 const h=harness(),old=h.current;h.click('/admin/tags');await h.finish(0,500);
 assert.equal(h.current,old);assert.equal(h.location.pathname,'/admin/catalog');assert.equal(h.message.textContent,'failed');
 assert.equal(h.retry.hidden,false);h.retry.listeners.click();await h.finish(1,401);
 assert.equal(h.login.hidden,false);assert.equal(h.retry.hidden,true);
});
test('dirty forms can cancel navigation and uploads cannot be interrupted by tabs',()=>{
 const h=harness();h.dirty=true;h.allow=false;h.click('/admin/tags');assert.equal(h.requests.length,0);
 h.allow=true;h.submitting=true;h.click('/admin/tags');assert.equal(h.requests.length,0);
 h.submitting=false;h.click('/admin/tags');assert.equal(h.requests.length,1);
});
test('browser history restores results without a new push; cancelled back restores current URL',async()=>{
 const h=harness();h.click('/admin/tags');await h.finish();h.pop('/admin/catalog?q=phone');await h.finish(1);
 assert.equal(h.location.search,'?q=phone');assert.equal(h.replaced,2);
 h.dirty=true;h.allow=false;h.pop('/admin/tags');assert.equal(h.location.pathname,'/admin/catalog');
});
test('remembered lists omit editor state, retain explicit binding context, and reject external URLs',()=>{
 const h=harness('http://localhost/admin/3d?q=phone&kind=model&target=m&name=iPhone&dialog=resource-upload&edit=old');
 const saved=JSON.parse(h.writes.at(-1))['/admin/3d'];
 assert.ok(saved.includes('target=m'));assert.ok(!saved.includes('dialog='));assert.ok(!saved.includes('edit='));
 const hostile=harness(undefined,JSON.stringify({'/admin/tags':'https://evil.test/admin/tags'}));
 hostile.click('/admin/tags');assert.equal(new URL(hostile.requests[0].url).origin,'http://localhost');
});
test('preview pages and modified clicks use native links; no fetched scripts execute',()=>{
 const h=harness();assert.equal(h.click('/admin/3d/resource',false),false);
 assert.equal(h.click('/admin/tags',true,true),false);assert.equal(h.requests.length,0);
 assert.match(source,/for \(const script of next.querySelectorAll\('script'\)\) script.remove\(\)/);
 assert.match(source,/document.addEventListener\('submit',[\s\S]*?}, true\)/);
});

test('typing during a pending switch cancels replacement and preserves the input surface',async()=>{
 const h=harness(),old=h.current;h.click('/admin/tags');h.input();await h.finish();
 assert.equal(h.current,old);assert.equal(h.replaced,0);assert.ok(h.requests[0].options.signal.aborted);
});
