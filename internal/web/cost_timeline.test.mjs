import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

function harness() {
  const doc = {listeners:{}, addEventListener(k,fn){this.listeners[k]=fn;}, activeElement:null};
  const items = [0,1].map(() => {
    const panel={hidden:true,inert:true};
    const trigger={listeners:{},attrs:{},setAttribute(k,v){this.attrs[k]=v;},addEventListener(k,fn){this.listeners[k]=fn;},focus(){doc.activeElement=this;}};
    return {panel,trigger,listeners:{},querySelector(s){return s==='.timeline-trigger'?trigger:panel;},contains(x){return x===this || x===trigger || x===panel;},addEventListener(k,fn){this.listeners[k]=fn;}};
  });
  doc.querySelectorAll=()=>items;
  let callback;
  vm.runInNewContext(readFileSync(new URL('./static/cost-timeline.js',import.meta.url),'utf8'),{document:doc,setTimeout(fn,ms){assert.equal(ms,150);callback=fn;return 1;},clearTimeout(){callback=null;}});
  return {items,doc,tick(){const fn=callback;callback=null;fn?.();}};
}

test('timeline delayed focus, cancellation and inline details',()=>{
  const {items:[a],tick}=harness();
  a.listeners.pointerenter({pointerType:'mouse'});assert.equal(a.panel.hidden,true);
  a.listeners.pointerleave();tick();assert.equal(a.panel.hidden,true);
  a.listeners.pointerenter({pointerType:'mouse'});tick();assert.equal(a.panel.hidden,false);
  assert.equal(a.trigger.attrs['aria-expanded'],'true');
  assert.equal(a.panel.inert,false);
  a.listeners.pointerleave();assert.equal(a.panel.hidden,true);
});

test('details expand inside the same record, with no detached popup',()=>{
  const template=readFileSync(new URL('./templates/asset.html',import.meta.url),'utf8');
  const css=readFileSync(new URL('./static/app.css',import.meta.url),'utf8');
  assert.ok(template.includes('class="timeline-details"'));
  assert.ok(template.includes('class="timeline-details-inner"'));
  assert.ok(!template.includes('timeline-popover'));
  assert.match(css,/\.timeline-details\s*\{[^}]*grid-template-rows:1fr/);
  assert.match(css,/\.timeline-details\[hidden\]\s*\{[^}]*grid-template-rows:0fr/);
  assert.match(css,/prefers-reduced-motion:reduce[^]*\.timeline-details/);
});

test('expanded record keeps a gutter clear of the timeline rail',()=>{
  const css=readFileSync(new URL('./static/app.css',import.meta.url),'utf8');
  assert.match(css,/\.compact-timeline \.timeline-entry\s*\{[^}]*margin-inline:0/);
  assert.match(css,/\.compact-timeline \.timeline-item\s*\{[^}]*gap:12px/);
  assert.match(css,/\.compact-timeline \.timeline-dot\s*\{[^}]*box-shadow:none/);
  assert.ok(!css.includes('transform:scale(1.045)'));
});
test('touch pins one item, outside click and Escape close',()=>{
  const {items:[a,b],doc,tick}=harness();
  a.listeners.pointerenter({pointerType:'touch'});tick();assert.equal(a.panel.hidden,true);
  a.trigger.listeners.click();a.listeners.pointerleave();assert.equal(a.panel.hidden,false);
  doc.listeners.click({target:a.panel});assert.equal(a.panel.hidden,false);
  b.trigger.listeners.click();assert.equal(a.panel.hidden,true);assert.equal(b.panel.hidden,false);
  doc.listeners.keydown({key:'Escape'});assert.equal(b.panel.hidden,true);assert.equal(doc.activeElement,b.trigger);
  a.trigger.listeners.click();doc.listeners.click({target:{}});assert.equal(a.panel.hidden,true);
  a.trigger.listeners.click();a.trigger.listeners.click();assert.equal(a.panel.hidden,true);
});
test('keyboard focus stays open inside the card',()=>{
  const {items:[a]}=harness();
  a.listeners.focusin();assert.equal(a.panel.hidden,false);
  a.listeners.focusout({relatedTarget:a.panel});assert.equal(a.panel.hidden,false);
  a.listeners.focusout({relatedTarget:{}});assert.equal(a.panel.hidden,true);
});
