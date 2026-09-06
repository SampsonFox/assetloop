import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const source=readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
function consume(href,id) {
  const block=source.match(/  const consumeDialogURL = \(dialog\) => \{[\s\S]*?\n  \};/);
  assert.ok(block,'shared dialog URL cleanup must exist');
  const state={preserved:true};
  let replaced;
  const context={URL,window:{location:{href},history:{state,replaceState(s,title,url){assert.equal(s,state);replaced=url;}}}};
  vm.runInNewContext(`${block[0]}\nconsumeDialogURL({id:${JSON.stringify(id)}});`,context);
  return replaced;
}
test('event deep link is consumed without losing unrelated filters',()=>{
  assert.equal(consume('http://localhost/assets/a?dialog=event-drawer&event_type=%E7%A7%9F%E8%B5%81&q=phone&sort=amount#add-event','event-drawer'),'/assets/a?q=phone&sort=amount#lifecycle-timeline');
});
test('legacy add-event hash is cleared but ordinary event filter survives',()=>{
  assert.equal(consume('http://localhost/assets/a?event_type=sale#add-event','event-drawer'),'/assets/a?event_type=sale#lifecycle-timeline');
});
test('unrelated dialogs and ordinary URLs are untouched',()=>{
  assert.equal(consume('http://localhost/assets/a?dialog=event-drawer#add-event','event-type-drawer'),undefined);
  assert.equal(consume('http://localhost/assets/a?event_type=sale#lifecycle-timeline','event-drawer'),undefined);
});
test('model edit deep links are consumed while preserving catalogue state',()=>{
  assert.equal(consume('http://localhost/admin/catalog?dialog=model-drawer&edit_model_id=abc&q=phone','model-drawer'),'/admin/catalog?q=phone');
});
test('both successful initial opening and actual close clean navigation state',()=>{
  assert.match(source,/opener\.click\(\);\s*consumeDialogURL/);
  assert.match(source,/addEventListener\("close", \(\) => \{\s*consumeDialogURL\(dialog\)/);
});
