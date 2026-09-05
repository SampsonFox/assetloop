import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

function harness() {
  const doc = {listeners:{}, addEventListener(k,fn){this.listeners[k]=fn;}, activeElement:null};
  const items = [0,1].map(() => {
    const panel={hidden:true};
    const trigger={listeners:{},attrs:{},setAttribute(k,v){this.attrs[k]=v;},addEventListener(k,fn){this.listeners[k]=fn;},focus(){doc.activeElement=this;}};
    return {panel,trigger,listeners:{},querySelector(s){return s==='.timeline-trigger'?trigger:panel;},contains(x){return x===this || x===trigger || x===panel;},addEventListener(k,fn){this.listeners[k]=fn;}};
  });
  doc.querySelectorAll=()=>items;
  let callback;
  vm.runInNewContext(readFileSync(new URL('./static/cost-timeline.js',import.meta.url),'utf8'),{document:doc,setTimeout(fn,ms){assert.equal(ms,150);callback=fn;return 1;},clearTimeout(){callback=null;}});
  return {items,doc,tick(){const fn=callback;callback=null;fn?.();}};
}

test('timeline delayed hover, cancellation and popover interaction',()=>{
  const {items:[a],tick}=harness();
  a.listeners.pointerenter({pointerType:'mouse'});assert.equal(a.panel.hidden,true);
  a.listeners.pointerleave();tick();assert.equal(a.panel.hidden,true);
  a.listeners.pointerenter({pointerType:'mouse'});tick();assert.equal(a.panel.hidden,false);
  assert.equal(a.trigger.attrs['aria-expanded'],'true');
  a.listeners.pointerleave();assert.equal(a.panel.hidden,true);
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
