import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
const read=p=>readFileSync(new URL(p,import.meta.url),'utf8');
test('shared rename confirms only actual changes and cancellation never submits',async()=>{
 const source=read('./static/app.js');
 for (const [name,accepted,promptCount,submitted] of [['A14',true,0,0],[' A14 ',true,0,0],['A14 corrected',false,1,0],['A14 corrected',true,1,1]]) {
  let handler,prompts=0,submits=0;
  const fields={name:{value:name},confirm_rename:{value:''}};
  const form={dataset:{sharedName:'A14',renameMessage:'Updates 2 references'},hasAttribute:()=>true,elements:{namedItem:n=>fields[n]},requestSubmit:()=>submits++};
  const ctx={document:{addEventListener:(_,fn)=>handler=fn},confirmDiscard:async(message,rename)=>{assert.equal(rename,true);assert.equal(message,'Updates 2 references');prompts++;return accepted;},window:{confirm:()=>true}};
  vm.runInNewContext(source.slice(source.indexOf('  document.addEventListener("submit", async'),source.indexOf('  window.addEventListener("beforeunload"')),ctx);
  await handler({target:form,preventDefault(){},defaultPrevented:false});
  assert.equal(prompts,promptCount);assert.equal(submits,submitted);
 }
});
test('no permanent rename checkbox; fallback retains explicit confirmation',()=>{
 const html=read('./templates/specifications.html');
 assert.doesNotMatch(html,/type="checkbox" name="confirm_rename"/);
 assert.match(html,/data-shared-name="{{\$data.OriginalName}}"/);
 assert.match(html,/<noscript>[\s\S]*?name="confirm_rename" value="1"/);
 assert.match(html,/type="hidden" name="confirm_rename" value="" disabled/);
});
test('resource editor keeps preview and all editable fields in one form with fixed heading save',()=>{
 const html=read('./templates/resource.html');
 assert.match(html,/template "resource-list"/);
 assert.match(html,/data-resource-editor/);
 assert.match(html,/type="submit" form="resource-form"/);
 const form=html.split('<form id="resource-form"')[1].split('</form>')[0];
 assert.doesNotMatch(form,/<form /);
 for (const field of ['resource_configuration','name','model_3d_author','tag_ids','category_ids']) assert.ok(form.includes('name="'+field+'"'),field);
 assert.match(form,/data-model-viewer/);
 assert.match(html,/form="resource-delete"/);
 assert.match(read('./static/app.css'), /\.resource-drawer-panel \.drawer-body \{ grid-auto-rows:max-content/);
});
