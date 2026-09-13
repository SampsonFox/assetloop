import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const source=readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
const start=source.indexOf('  const syncEventTypeFields =');
assert.notEqual(start,-1,'event type amount policy lives in syncEventTypeFields');
const block=source.slice(start,source.indexOf('  const formBaselines =',start));

// The fake amount field mirrors the real control: named input plus the data
// attributes the form renders for the zero-allowed purchase policy.
function harness() {
  const amount={value:'',required:false,dataset:{nonnegativePattern:'NONNEG',nonnegativeTitle:'nonneg title',positivePattern:'POS',positiveTitle:'pos title'},removeAttribute(){delete this.pattern;}};
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
  const use=(cashflow,systemCode)=>{select.selectedOptions=[{dataset:{cashflow,systemCode}}];context.sync(select);};
  return {amount,moneyFields,use};
}

test('a zero purchase is accepted as a base-currency gift',()=>{
  const h=harness();
  h.amount.value='0';
  h.use('expense','purchase');
  assert.equal(h.amount.value,'0');
  assert.equal(h.amount.required,true);
  assert.equal(h.amount.pattern,'NONNEG');
  assert.equal(h.amount.title,'nonneg title');
});

test('built-in repair stays strictly positive',()=>{
  const h=harness();
  h.amount.value='0';
  h.use('expense','repair');
  assert.equal(h.amount.value,'');
  assert.equal(h.amount.required,true);
  assert.equal(h.amount.pattern,'POS');
  assert.equal(h.moneyFields[0].hidden,false);
});

test('switching neutral back to purchase restores the previous amount instead of gifting it',()=>{
  const h=harness();
  h.amount.value='500.00';
  h.use('expense','purchase');
  h.use('neutral','');
  assert.equal(h.amount.value,'0');
  assert.equal(h.amount.pattern,undefined);
  assert.equal(h.moneyFields[0].hidden,true);
  h.use('expense','purchase');
  assert.equal(h.amount.value,'500.00');
  assert.equal(h.amount.pattern,'NONNEG');
  assert.equal(h.moneyFields[0].hidden,false);
});
