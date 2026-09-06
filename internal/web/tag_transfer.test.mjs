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
 assert.match(html,/tags.save_allowed/);
});
test('enhancement supports repeated settings loads without duplicate widgets, search does not submit',()=>{
 const js=readFileSync(new URL('./static/tag-transfer.js',import.meta.url),'utf8');
 assert.match(js,/initialized.has\(form\)/);assert.match(js,/settings:loaded/);
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
