import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import vm from 'node:vm';

const source=process.env.TEST_OLD_DIALOG ? execFileSync('git',['-c',`safe.directory=${process.cwd().replaceAll('\\','/')}`,'show','HEAD:internal/web/static/app.js'],{encoding:'utf8'}) : readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
function harness(forms) {
  const start=source.indexOf('  const formBaselines =');
  assert.notEqual(start,-1,'compare actual values to opening baseline, not input-event flags');
  const state=source.slice(start,source.indexOf('  // Use an in-page modal:',start));
  const close=source.slice(source.indexOf('  const closingDialogs ='),source.indexOf('  const focusDialog ='));
  let answer=true, prompts=0, resets=0, pending;
  const dialog={open:true,querySelectorAll:()=>forms,close(){this.open=false;}};
  const context={confirmDiscard:()=>{prompts++;return pending || Promise.resolve(answer);},discardDialogForms:()=>{resets++;}};
  vm.createContext(context);
  vm.runInContext(`${state}\n${close}\nthis.api={rememberDialogForms,updateDirty,dirtyForm,closeDialog};`,context);
  context.api.rememberDialogForms(dialog);
  return {...context.api,dialog,get prompts(){return prompts;},get resets(){return resets;},answer(v){answer=v;},pending(v){pending=v;}};
}
const field=(name,value,type='text')=>({name,value,type});
const form=(elements)=>({elements,dataset:{discardConfirm:'Discard?'}});

test('unchanged create/edit closes directly; changing back to original does not prompt',async()=>{
  const f=form([field('name','iPhone')]), h=harness([f]);
  f.elements[0].value='other';h.updateDirty(f);assert.equal(f.dataset.dirty,'true');
  f.elements[0].value='iPhone';h.updateDirty(f);assert.equal(f.dataset.dirty,'false');
  assert.equal(await h.closeDialog(h.dialog),true);assert.equal(h.prompts,0);assert.equal(h.dialog.open,false);
});
test('dirty secondary form prompts, keep editing preserves it, discard closes once',async()=>{
  const f=form([field('name','original')]), second=form([{name:'tag_ids',type:'checkbox',checked:true}]);
  const h=harness([f,second]);second.elements[0].checked=false;
  h.answer(false);assert.equal(await h.closeDialog(h.dialog),false);
  assert.equal(h.dialog.open,true);assert.equal(second.elements[0].checked,false);assert.equal(h.resets,0);
  h.answer(true);assert.equal(await h.closeDialog(h.dialog),true);assert.equal(h.resets,1);
});
test('duplicate close requests cannot stack confirmations',async()=>{
  const f=form([field('name','original')]), h=harness([f]);f.elements[0].value='changed';
  let resolve;h.pending(new Promise(r=>resolve=r));
  const first=h.closeDialog(h.dialog);assert.equal(await h.closeDialog(h.dialog),false);
  assert.equal(h.prompts,1);resolve(true);await first;assert.equal(h.resets,1);
});
test('search and temporary checks are ignored; transfer fields and file changes are guarded',()=>{
  const f=form([field('name','original'),field('','search'),{name:'upload',type:'file',files:[]}]);
  const h=harness([f]);f.elements[1].value='CPU';f.dataset.dirty='true';
  assert.equal(h.dirtyForm(h.dialog),undefined);
  f.elements[2].files=[{name:'model.glb',size:42,lastModified:1}];assert.equal(h.dirtyForm(h.dialog),f);
});
test('all dirty flags are recomputed, even when first form is dirty',()=>{
  const a=form([field('name','a')]), b=form([field('name','b')]), h=harness([a,b]);
  a.elements[0].value='changed';b.dataset.dirty='true';h.dirtyForm(h.dialog);assert.equal(b.dataset.dirty,'false');
});
test('cancel, X, backdrop and Escape share close path; prompt is page-owned and accessible',()=>{
  assert.match(source,/if \(closer\) \{\s*event.preventDefault\(\);\s*closeDialog\(closer.closest\("dialog"\)\)/);
  assert.match(source,/event.target.matches\("dialog.drawer"\)\) \{\s*closeDialog\(event.target\)/);
  assert.match(source,/dialog.addEventListener\("cancel", \(event\) => \{\s*event.preventDefault\(\);\s*closeDialog\(dialog\)/);
  const prompt=source.slice(source.indexOf('  const confirmDiscard'),source.indexOf('  // Deep links'));
  assert.doesNotMatch(prompt,/window.confirm/);assert.match(prompt,/aria-labelledby/);assert.match(prompt,/stay.focus\(\)/);
  assert.match(prompt,/Keep editing/);assert.match(prompt,/放弃修改/);
});
