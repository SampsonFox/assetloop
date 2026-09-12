import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import vm from 'node:vm';

const source=process.env.TEST_OLD_DIALOG ? execFileSync('git',['-c',`safe.directory=${process.cwd().replaceAll('\\','/')}`,'show','HEAD:internal/web/static/app.js'],{encoding:'utf8'}) : readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
test('opening a lifecycle form retains its default type unless the opener supplies one',()=>{
 const start=source.indexOf('      for (const [dataKey, fieldName] of Object.entries(fields))');
 const loop=source.slice(start,source.indexOf('      if (dialog.id === "model-drawer")',start));
 const field={value:'buy-id'},context={fields:{eventType:'event_type'},form:{elements:{namedItem:()=>field}},opener:{dataset:{}}};
 vm.runInNewContext(loop,context); assert.equal(field.value,'buy-id');
 context.opener.dataset.eventType='custom-id';vm.runInNewContext(loop,context);assert.equal(field.value,'custom-id');
});
function harness(forms) {
  const start=source.indexOf('  const formBaselines =');
  assert.notEqual(start,-1,'compare actual values to opening baseline, not input-event flags');
  const state=source.slice(start,source.indexOf('  // Use an in-page modal:',start));
  const close=source.slice(source.indexOf('  const closingDialogs ='),source.indexOf('  // Standalone editors'));
  let answer=true, prompts=0, resets=0, pending;
  const dialog={open:true,dataset:{},querySelector:()=>null,hasAttribute:()=>false,querySelectorAll:()=>forms,close(){this.open=false;}};
  const context={window:{},confirmDiscard:()=>{prompts++;return pending || Promise.resolve(answer);},discardDialogForms:()=>{resets++;}};
  vm.createContext(context);
  vm.runInContext(`${state}\n${close}\nthis.api={rememberDialogForms,updateDirty,dirtyForm,closeDialog};`,context);
  context.api.rememberDialogForms(dialog);
  return {...context.api,dialog,get prompts(){return prompts;},get resets(){return resets;},answer(v){answer=v;},pending(v){pending=v;}};
}
const field=(name,value,type='text')=>({name,value,type});

test('native submit preserves the decision button and blocks repeated submission',async()=>{
 const start=source.indexOf('  document.addEventListener("submit", async (event) => {');
 const block=source.slice(start,source.indexOf('  window.addEventListener("beforeunload"',start));
 let submit;
 vm.runInNewContext(block,{document:{addEventListener:(_name,handler)=>{submit=handler;}},window:{}});
 for(const value of ['allow','deny']) {
  const target={dataset:{},hasAttribute:()=>false};
  const submitter={name:'decision',value,disabled:false,setAttribute(){}};
  let prevented=false;
  const event={target,submitter,preventDefault(){prevented=true;}};
  await submit(event);
  assert.equal(prevented,false);
  assert.equal(submitter.disabled,false,'disabled successful controls lose their name/value');
  assert.equal(submitter.value,value);
  await submit(event);
  assert.equal(prevented,true);
 }
});
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

test('standalone editor navigation confirms once, preserves rejected drafts and bypasses duplicate unload prompt',async()=>{
  const block=source.slice(source.indexOf('  const pageForms ='),source.indexOf("  document.addEventListener('click', (event) =>"));
  assert.ok(block.includes('leaveEditor'));
  const form={dataset:{dirty:'true',discardConfirm:'Discard?'},closest:()=>null};
  let answer=false,calls=0;const navigated=[];
  const context={document:{querySelectorAll:()=>[form]},updateDirty:()=>{},confirmDiscard:async()=>{calls++;return answer;},window:{location:{assign:url=>navigated.push(url)}}};
  vm.createContext(context);vm.runInContext(`${block}\nthis.leave=leaveEditor;this.leaving=()=>leavingPage;`,context);
  await context.leave('/');assert.deepEqual(navigated,[]);assert.equal(context.leaving(),false);
  answer=true;await context.leave('/');assert.deepEqual(navigated,['/']);assert.equal(context.leaving(),true);
  await context.leave('/');assert.equal(calls,2);
  assert.match(source,/if \(leavingPage\) return;/);
});
test('lifecycle dropdown creation restores the selection before opening a child and returns focus',()=>{
  const block=source.slice(source.indexOf('  const chooseEventType ='),source.indexOf('  const syncEventTypeFields ='));
  const select={value:'__create__',dataset:{selectedType:'buy-id'},selectedOptions:[{hasAttribute:()=>true}],isConnected:true,focus:()=>{focused=true;}};
  let opened,focused=false;
  const context={window:{assetloopDrawers:{open:link=>{assert.equal(select.value,'buy-id');opened=link;}}},select};
  vm.createContext(context);vm.runInContext(`${block}\nthis.choose=chooseEventType;`,context);
  assert.equal(context.choose(select),true);
  assert.equal(opened.href,'/admin/event-types?dialog=event-type-manage');
  assert.equal(opened.dataset.drawerTarget,'event-type-manage');
  opened.focus();assert.equal(focused,true);
  select.selectedOptions[0].hasAttribute=()=>false;select.value='new-id';
  assert.equal(context.choose(select),false);assert.equal(select.value,'new-id');
  const template=readFileSync(new URL('templates/asset.html',import.meta.url),'utf8');
  assert.match(template,/data-event-type-create hidden disabled/);
  assert.doesNotMatch(template,/types.view_selected|data-drawer-query="edit_type_id"/);
});

test('unchanged standalone editor leaves without asking',async()=>{
  const block=source.slice(source.indexOf('  const pageForms ='),source.indexOf("  document.addEventListener('click', (event) =>"));
  let navigated=false;
  const context={document:{querySelectorAll:()=>[{dataset:{dirty:'false'},closest:()=>null}]},updateDirty:()=>{},confirmDiscard:()=>assert.fail('unexpected prompt'),window:{location:{assign:()=>{navigated=true;}}}};
  vm.createContext(context);vm.runInContext(`${block}\nthis.leave=leaveEditor;`,context);await context.leave('/');assert.equal(navigated,true);
});

test('SSR step drawers retain server-rendered fields and action without opener metadata',()=>{
 const start=source.indexOf('      if (opener.dataset.action) form.action');
 assert.notEqual(start,-1);
 const block=source.slice(start,source.indexOf('      if (dialog.id === "model-drawer")',start));
 const field={value:'Phone 256GB'},form={action:'/admin/market/discover',hasAttribute:()=>true,elements:{namedItem:()=>field}};
 vm.runInNewContext(block,{fields:{name:'name'},form,opener:{dataset:{}},dialog:{querySelector:()=>null}});
 assert.equal(form.action,'/admin/market/discover');assert.equal(field.value,'Phone 256GB');
});
