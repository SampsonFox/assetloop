import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const source=readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
const start=source.indexOf('  const syncEventTypeFields =');
assert.notEqual(start,-1,'event type amount policy lives in syncEventTypeFields');
const block=source.slice(start,source.indexOf('  const formBaselines =',start));

// The fake amount field mirrors the real control: a named input plus the
// positive-amount pattern the form renders for every expense or income type.
function harness() {
  const amount={value:'',required:false,dataset:{positivePattern:'POS'},removeAttribute(){delete this.pattern;}};
  const moneyFields=[{hidden:false}];
  const form={
    elements:{namedItem:(name)=>name==='amount'?amount:null},
    querySelectorAll:(selector)=>selector==='[data-money-field]'?moneyFields:[],
    querySelector:()=>null,
  };
  const select={value:'',dataset:{},form,selectedOptions:[],querySelector:()=>null};
  const context={};
  vm.createContext(context);
  vm.runInContext(`${block}\nthis.sync=syncEventTypeFields;`,context);
  const use=(cashflow)=>{select.selectedOptions=[{dataset:{cashflow}}];context.sync(select);};
  return {amount,moneyFields,use};
}

test('a purchase keeps the strictly positive amount policy',()=>{
  const h=harness();
  h.amount.value='0';
  h.use('expense');
  assert.equal(h.amount.value,'');
  assert.equal(h.amount.required,true);
  assert.equal(h.amount.pattern,'POS');
  assert.equal(h.moneyFields[0].hidden,false);
});

test('a repair keeps the strictly positive amount policy',()=>{
  const h=harness();
  h.amount.value='0';
  h.use('expense');
  assert.equal(h.amount.value,'');
  assert.equal(h.amount.required,true);
  assert.equal(h.amount.pattern,'POS');
  assert.equal(h.moneyFields[0].hidden,false);
});

test('a custom neutral type records zero and switching back restores the previous amount',()=>{
  const h=harness();
  h.amount.value='500.00';
  h.use('expense');
  h.use('neutral');
  assert.equal(h.amount.value,'0');
  assert.equal(h.amount.pattern,undefined);
  assert.equal(h.amount.required,false);
  assert.equal(h.moneyFields[0].hidden,true);
  h.use('expense');
  assert.equal(h.amount.value,'500.00');
  assert.equal(h.amount.pattern,'POS');
  assert.equal(h.moneyFields[0].hidden,false);
});
