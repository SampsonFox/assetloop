import test from 'node:test';
import {selectTransferRows} from './static/tag-transfer-state.mjs';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {createTagTransferState} from './static/tag-transfer-state.mjs';
const dimensions=()=>[
 {id:'color',name:'颜色',appearance:true,override:'',choices:[{id:'black',name:'黑色',enabled:true,selected:true},{id:'white',name:'白色',enabled:true}]},
 {id:'storage',name:'储存',appearance:false,override:'',choices:[{id:'128',enabled:true},{id:'256',enabled:true,selected:true}]},
 {id:'cpu',name:'CPU 型号',appearance:false,override:'',choices:[{id:'a19',enabled:true}]},
];
test('transfer names and detail actions stay in one non-wrapping row',()=>{
 const css=readFileSync(new URL('./static/app.css',import.meta.url),'utf8');
 const rows=[...css.matchAll(/^\.transfer-row\s*\{([^}]+)\}/gm)];
 const choices=[...css.matchAll(/\.transfer-row \.transfer-choice\s*\{([^}]+)\}/g)];
 assert.match(rows.at(-1)[1],/flex-wrap:nowrap/);
 assert.match(choices.at(-1)[1],/flex:0 1 auto/);
 assert.match(choices.at(-1)[1],/justify-content:flex-start/);
 assert.match(choices.at(-1)[1],/width:auto/);
 assert.match(css,/\.transfer-detail\s*\{[^}]*flex:0 0 auto/);
 assert.match(css,/\.transfer-group-title::after\s*\{[^}]*border-top:1px solid var\(--line\)/);
 assert.match(css,/\.transfer-group > \.transfer-row\s*\{[^}]*padding-inline-start:8px/);
});
test('dynamic dimensions activate only after choosing a value; defaults are inherited',()=>{
 const s=createTagTransferState(dimensions());assert.deepEqual(s.active().map(d=>d.id),['color','storage']);
 s.moveTags(['a19'],true);assert.deepEqual(s.active().map(d=>d.id),['color','storage','cpu']);
 assert.equal(s.affects(s.active()[0]),true);assert.equal(s.affects(s.active()[2]),false);
});
test('bulk moves are idempotent and single-choice item types still allow multiple model values',()=>{
 const s=createTagTransferState(dimensions());s.moveTags(['128','256','128'],true);
 assert.equal(s.selected.size,3);assert.ok(s.selected.has('128')&&s.selected.has('256'));
 s.moveTags(['128','256'],false);assert.equal(s.selected.size,1);
});
test('moving dimensions creates model overrides; reset restores actual type default',()=>{
 const ds=dimensions(),s=createTagTransferState(ds);s.moveAppearance(['color'],false);
 assert.equal(s.overrides.get('color'),'no');assert.equal(ds[0].appearance,true);assert.equal(s.affects(ds[0]),false);
 s.resetAppearance('color');assert.equal(s.affects(ds[0]),true);assert.equal(s.overrides.size,0);
});
test('removing last value removes dimension override, re-add follows the default',()=>{
 const ds=dimensions(),s=createTagTransferState(ds);s.moveAppearance(['color'],false);s.moveTags(['black'],false);
 assert.equal(s.overrides.size,0);s.moveTags(['white'],true);assert.equal(s.affects(ds[0]),true);
});
test('missing and inactive IDs cannot create choices or appearance overrides',()=>{
 const s=createTagTransferState(dimensions());s.moveTags(['missing'],true);s.moveAppearance(['cpu','missing'],true);
 assert.equal(s.selected.size,2);assert.equal(s.overrides.size,0);
});
test('disabled retained choices can be undone, never newly selected',()=>{
 const ds=dimensions();ds[0].choices[0].enabled=false;ds[0].choices[1].enabled=false;
 const s=createTagTransferState(ds);s.moveTags(['black'],false);s.moveTags(['black','white'],true);
 assert.ok(s.selected.has('black'));assert.ok(!s.selected.has('white'));
});
test('native form retains stable IDs and override fields without JavaScript',()=>{
 const html=readFileSync(new URL('./templates/catalog_drawers.html',import.meta.url),'utf8');
 assert.match(html,/data-transfer-source/);assert.match(html,/data-tag-dimension="{{.ID}}"/);
 assert.match(html,/name="tag_ids"/);assert.match(html,/name="appearance_{{.ID}}"/);
 assert.match(html,/name="model_configuration" value="1"/);
 assert.match(html,/form="model-form"/);
});
test('enhancement supports repeated settings loads without duplicate widgets, search does not submit',()=>{
 const js=readFileSync(new URL('./static/tag-transfer.js',import.meta.url),'utf8');
 assert.match(js,/initialized.has\(editor\)/);assert.match(js,/settings:loaded/);
 assert.match(js,/event.key==='Enter'\) event.preventDefault\(\)/);
 assert.match(js,/form.dataset.dirty=String\(dirty\)/);
 assert.match(js,/source.hidden=true/);
});

test('discarding the shared drawer resets every dirty form and rebuilds both transfers',()=>{
 const js=readFileSync(new URL('./static/tag-transfer.js',import.meta.url),'utf8');
 const app=readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
 assert.match(js,/form.addEventListener\('reset'/);
 assert.match(js,/state=createTagTransferState\(dimensions\);sync\(false\)/);
 assert.match(app,/for \(const form of dialog.querySelectorAll\("form\[data-guard-dirty\]"\)\)/);
 assert.match(app,/if \(form.dataset.dirty === "true"\) resetForm\(form\)/);
});

test('plain click replaces, Ctrl/Cmd toggles and Shift selects the visible range',()=>{
 const selected=new Set();const rows=['a','b','c'];
 let anchor=selectTransferRows(selected,rows,'a',null);assert.deepEqual([...selected],['a']);
 anchor=selectTransferRows(selected,rows,'c',anchor,{toggle:true});assert.deepEqual([...selected],['a','c']);
 anchor=selectTransferRows(selected,rows,'a',anchor,{toggle:true});assert.deepEqual([...selected],['c']);
 anchor=selectTransferRows(selected,rows,'a',anchor);assert.deepEqual([...selected],['a']);
 selectTransferRows(selected,rows,'c',anchor,{range:true});assert.deepEqual([...selected],rows);
 selectTransferRows(selected,rows,'missing',anchor);assert.deepEqual([...selected],rows);
});
test('temporary row selections do not mutate persisted tag/default state',()=>{
 const state=createTagTransferState(dimensions()), selected=new Set();
 selectTransferRows(selected,['black','white'],'white',null);
 assert.ok(!state.selected.has('white'));assert.equal(state.overrides.size,0);
});
test('row transfer uses central icon arrows, double click and keyboard without checkboxes',()=>{
 const js=readFileSync(new URL('./static/tag-transfer.js',import.meta.url),'utf8');
 const component=js.slice(js.indexOf('function transfer('),js.indexOf('function initialize('));
 assert.doesNotMatch(component,/type='checkbox'|transfer-footer/);
 assert.match(component,/transfer-arrows/);assert.match(component,/dblclick/);
 for(const key of ['ArrowDown','ArrowUp','Enter','Home','End'])assert.ok(component.includes(key));
 assert.match(component,/pointerType==='touch'/);assert.match(component,/aria-pressed/);
 assert.match(component,/selections\[i\].clear\(\);anchors\[i\]=null;render/);
});

test('reset all appearance overrides preserves tag allowances and follows each type default',()=>{
 const state=createTagTransferState(dimensions());const before=[...state.selected];
 state.moveAppearance(['color'],false);state.overrides.set('storage','yes');
 state.resetAllAppearance();assert.equal(state.overrides.size,0);assert.deepEqual([...state.selected],before);
 for(const d of state.active())assert.equal(state.affects(d),d.appearance);
});
test('one reset action belongs to the dimension heading; only default-affecting types carry a marker',()=>{
 const js=readFileSync(new URL('./static/tag-transfer.js',import.meta.url),'utf8');
 assert.match(js,/if\(reset\)\{[\s\S]*?heading.append\(restore\)/);
 assert.match(js,/restore.title=text.reset;restore.setAttribute\('aria-label',text.reset\)/);
 assert.doesNotMatch(js,/row.append\(button\(text.reset/);
 assert.match(js,/note:d.appearance\?text.inherited:''/);
 assert.match(js,/state.resetAllAppearance\(\)/);
});

test('transfer boundaries pass scrolling to the drawer, while the drawer contains background scrolling',()=>{
 const css=readFileSync(new URL('./static/app.css',import.meta.url),'utf8');
 const rule=css.match(/\.transfer-list\s*\{([^}]+)\}/)[1];
 assert.match(rule,/overscroll-behavior:\s*auto/);
 assert.match(css,/\.drawer-panel,\.drawer-body\s*\{\s*overscroll-behavior:contain/);
});
