import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

function harness({accept=true}={}) {
  const choice = (value,checked=false) => ({value,checked,tagName:'INPUT',dataset:{tagName:value}});
  let current=[choice('black',true),choice('256GB',true)];
  const fields={querySelectorAll(){return current.filter(x=>x.checked);},replaceChildren(next){current=next?.choices || [];}};
  const model={value:'phone',dataset:{removeTagsConfirm:'Remove incompatible tags?'},addEventListener(_,fn){this.change=fn;}};
  const templates=['phone','tablet'].map(id=>({dataset:{assetTagTemplate:id},content:{cloneNode(){const choices=(id==='phone'?['black','256GB']:['black','512GB']).map(x=>choice(x));return {choices,querySelectorAll(selector){return selector==='select'?[]:choices;}};}}}));
  const confirmations=[];
  vm.runInNewContext(readFileSync(new URL('./static/asset-tags.js',import.meta.url),'utf8'),{
    document:{querySelector:s=>s==='[data-asset-model]'?model:fields,querySelectorAll:()=>templates},
    window:{confirm(message){confirmations.push(message);return accept;}},
  });
  return {model,confirmations,get current(){return current;},switchTo(id){model.value=id;model.change();}};
}

test('model switch preserves allowed selections and confirms the exact removed labels',()=>{
  const h=harness();h.switchTo('tablet');
  assert.deepEqual(h.current.filter(x=>x.checked).map(x=>x.value),['black']);
  assert.equal(h.confirmations.length,1);assert.match(h.confirmations[0],/256GB/);
  assert.doesNotMatch(h.confirmations[0],/black/);
});
test('cancelled switch keeps the original model and controls intact',()=>{
  const h=harness({accept:false});const before=h.current;h.switchTo('tablet');
  assert.equal(h.model.value,'phone');assert.equal(h.current,before);
});
test('newly available tags are never auto-selected and all choices can be cleared',()=>{
  const h=harness();h.switchTo('tablet');assert.equal(h.current.find(x=>x.value==='512GB').checked,false);
  h.switchTo('');assert.equal(h.current.length,0);
  h.switchTo('phone');assert.equal(h.current.filter(x=>x.checked).length,0);
});
test('no tag picker leaves unrelated pages untouched',()=>{
  vm.runInNewContext(readFileSync(new URL('./static/asset-tags.js',import.meta.url),'utf8'),{document:{querySelector(){return null;}}});
});
