import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const choice = (value,checked=false) => ({value,checked,tagName:'INPUT',dataset:{tagName:value}});
test('searchable resource choice changes only the parent draft and previews the selected ID',()=>{
  const source=readFileSync(new URL('./static/asset-tags.js',import.meta.url),'utf8');
  const block=source.slice(source.indexOf('  function initializeResource'),source.indexOf('  function initializeModel'));
  const nodes=[];
  const node=()=>({value:'',validity:{valid:true},setAttribute(){},removeAttribute(){},append(){},addEventListener(k,v){this[k]=v;},setCustomValidity(v){this.validity.valid=!v;}});
  const preview=node(); let opened,changes=0;
  const form={addEventListener(){}};
  const select={id:'resource',value:'',dataset:{invalidMessage:'Choose from list'},options:[{value:'',text:'Inherit'},{value:'id-1',text:'Phone'}],form,closest:()=>({querySelector:()=>preview}),removeAttribute(){},before(){},dispatchEvent(){changes++;}};
  const context={select,Event,queueMicrotask,document:{createElement(){const n=node();nodes.push(n);return n;}},window:{assetloopDrawers:{open:link=>{opened=link;}}}};
  vm.runInNewContext(`${block}\ninitializeResource(select)`,context);
  const input=nodes[0];assert.equal(input.value,'Inherit');assert.equal(preview.disabled,true);
  input.value='Phone';input.input();assert.equal(select.value,'id-1');assert.equal(changes,1);
  preview.click();assert.equal(opened.href,'/admin/3d/id-1');assert.equal(select.value,'id-1');
  input.value='Unknown';input.input();assert.equal(input.validity.valid,false);assert.equal(preview.disabled,true);
  input.value='Inherit';input.input();assert.equal(select.value,'');assert.equal(input.validity.valid,true);
});
function tagTemplate(id, values) {
  return {dataset:{assetTagTemplate:id},content:{cloneNode(){
    const choices=values.map(x=>choice(x));
    return {choices,querySelectorAll(selector){return selector==='select'?[]:choices;},append(node){
      const collect=node=>{if(node.tagName==='INPUT') choices.push(node);for(const child of node.children||[])collect(child);};collect(node);
    }};
  }}};
}
function editor() {
  let current=[choice('black',true),choice('256GB',true)];
  const fields={querySelectorAll(){return current.filter(x=>x.checked);},replaceChildren(next){current=next?.choices || [];}};
  const model={value:'phone',isConnected:true,dataset:{removeTagsConfirm:'Remove incompatible tags?'},listeners:0,addEventListener(_,fn){this.change=fn;this.listeners++;},dispatchEvent(){return this.change();},closest(){return form;}};
  const templates=[tagTemplate('phone',['black','256GB']),tagTemplate('tablet',['black','512GB'])];
  const install=template=>{template.replaceWith=next=>{const index=templates.indexOf(template);install(next);templates.splice(index,1,next);};};
  templates.forEach(install);
  const status={textContent:''}, draft={notes:'Unsent parent notes',displayName:'Unsent name'};
  const form={querySelector:s=>s==='[data-tag-change-status]'?status:fields,querySelectorAll:()=>templates,append(next){install(next);templates.push(next);}};
  return {model,draft,status,templates,querySelectorAll:selector=>selector==='[data-asset-model]'?[model]:[],get current(){return current;},switchTo(id){model.value=id;return model.change();}};
}
function harness({accept=true, fetchResponse}={}) {
  const parent=editor(), confirmations=[], listeners={}, requests=[], editors=[parent];
  const document={documentElement:{lang:'en'},querySelectorAll:selector=>selector==='[data-asset-model]'?editors.map(x=>x.model):[],addEventListener(type,fn){listeners[type]=fn;},createElement(tag){return {tagName:tag.toUpperCase(),dataset:{},children:[],append(...children){this.children.push(...children);}};},createTextNode(text){return {textContent:text};}};
  const window={assetloopDialog:{async confirm(message){confirmations.push(message);return typeof accept==='function'?accept():accept;}}};
  const source=readFileSync(new URL('./static/asset-tags.js',import.meta.url),'utf8');
  const context=vm.createContext({document,window,Event,fetch:async(url,options)=>{requests.push({url,options});return fetchResponse();},DOMParser:class{parseFromString(template){return {querySelectorAll:()=>[template]};}}});
  vm.runInContext(source,context);
  return {parent,confirmations,requests,save(kind,id){return listeners['drawer:saved']({detail:{kind,id}});},load(child){if(!editors.includes(child))editors.push(child);listeners['drawer:loaded']({target:child});},reload(){vm.runInContext(source,context);}};
}

test('model switch preserves allowed selections and confirms the exact removed labels',async()=>{
  const h=harness();await h.parent.switchTo('tablet');
  assert.deepEqual(h.parent.current.filter(x=>x.checked).map(x=>x.value),['black']);
  assert.equal(h.confirmations.length,1);assert.match(h.confirmations[0],/256GB/);
  assert.doesNotMatch(h.confirmations[0],/black/);
});
test('cancelled switch keeps the original model and controls intact',async()=>{
  const {parent}=harness({accept:false});const before=parent.current;await parent.switchTo('tablet');
  assert.equal(parent.model.value,'phone');assert.equal(parent.current,before);
});
test('newly available tags are never auto-selected and all choices can be cleared',async()=>{
  const {parent}=harness();await parent.switchTo('tablet');assert.equal(parent.current.find(x=>x.value==='512GB').checked,false);
  await parent.switchTo('');assert.equal(parent.current.length,0);
  await parent.switchTo('phone');assert.equal(parent.current.filter(x=>x.checked).length,0);
});
test('dynamic drawers initialize once and never reset the parent draft',async()=>{
  const h=harness(), child=editor(), before=h.parent.current;
  h.load(child);h.load(child);h.reload();
  assert.equal(child.model.listeners,1);assert.equal(h.parent.model.listeners,1);
  await child.switchTo('tablet');
  assert.equal(h.parent.current,before);assert.equal(h.parent.model.value,'phone');
  assert.equal(h.parent.current.find(x=>x.value==='256GB').checked,true);
  assert.deepEqual(child.current.filter(x=>x.checked).map(x=>x.value),['black']);
});
test('an older confirmation cannot replace a newer model selection',async()=>{
  let resolve;
  const h=harness({accept:()=>new Promise(done=>{resolve=done;})});
  const before=h.parent.current;
  const pending=h.parent.switchTo('tablet');
  await h.parent.switchTo('phone');resolve(true);await pending;
  assert.equal(h.parent.model.value,'phone');assert.equal(h.parent.current,before);
});
test('saved model refresh merges choices while retaining invalid selections and the parent draft',async()=>{
  const fresh=tagTemplate('phone',['black','1TB']);
  const h=harness({fetchResponse:async()=>({ok:true,text:async()=>fresh})});
  const draft=h.parent.draft, other=editor();other.model.value='tablet';h.load(other);
  const otherBefore=other.current;
  await h.save('tag','phone');await h.save('model','unselected');
  assert.equal(h.requests.length,0);
  await h.save('model','phone');
  assert.equal(h.requests.length,1);assert.equal(h.requests[0].url,'/assets/new?model_id=phone');
  assert.equal(h.requests[0].options.headers['X-Assetloop-Drawer'],'asset-editor');
  assert.deepEqual(h.parent.current.filter(x=>x.checked).map(x=>x.value),['black','256GB']);
  assert.equal(h.parent.current.find(x=>x.value==='1TB').checked,false);
  assert.equal(h.parent.current.find(x=>x.value==='256GB').name,'tag_ids');
  assert.match(h.parent.status.textContent,/Uncheck.*unavailable/);
  assert.equal(h.parent.draft,draft);assert.equal(draft.notes,'Unsent parent notes');
  assert.equal(other.current,otherBefore);assert.equal(h.parent.templates[0],fresh);
});
test('failed model refresh preserves every existing control and reports failure',async()=>{
  const h=harness({fetchResponse:async()=>({ok:false})}), before=h.parent.current;
  await h.save('model','phone');
  assert.equal(h.parent.current,before);assert.match(h.parent.status.textContent,/Unable to refresh/);
});
test('a model refresh arriving after a selection change cannot replace that selection',async()=>{
  let resolve;
  const h=harness({fetchResponse:()=>new Promise(done=>{resolve=done;})});
  const pending=h.save('model','phone');
  await h.parent.switchTo('tablet');const before=h.parent.current;
  resolve({ok:true,text:async()=>tagTemplate('phone',['black','1TB'])});await pending;
  assert.equal(h.parent.model.value,'tablet');assert.equal(h.parent.current,before);
});
test('no tag picker leaves unrelated pages untouched',()=>{
  vm.runInNewContext(readFileSync(new URL('./static/asset-tags.js',import.meta.url),'utf8'),{window:{},document:{querySelectorAll(){return [];},addEventListener(){}}});
});
