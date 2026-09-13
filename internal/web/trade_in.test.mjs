import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {tradeInDirectionForSystemCode, tradeInFXVisible, tradeInNavigationTarget} from './static/trade-in-state.mjs';

test('a pairing system type opens the dedicated trade-in form and never the ordinary record route',()=>{
 assert.equal(tradeInDirectionForSystemCode('trade_in_source'),'source');
 assert.equal(tradeInDirectionForSystemCode('trade_in_destination'),'destination');
 assert.equal(tradeInDirectionForSystemCode('purchase'),'');
 assert.equal(tradeInDirectionForSystemCode(''),'');
 for(const code of ['purchase','repair','sale','trade_in_other'])assert.equal(tradeInNavigationTarget('/assets/a/trade-in',code),'');
 assert.equal(tradeInNavigationTarget('/assets/a/trade-in','trade_in_source'),'/assets/a/trade-in?direction=source');
 assert.equal(tradeInNavigationTarget('/assets/a/trade-in','trade_in_destination'),'/assets/a/trade-in?direction=destination');
 assert.equal(tradeInNavigationTarget('','trade_in_source'),'');
});

test('the enhanced selector switches on the option system code and keeps the ordinary form intact',()=>{
 const js=readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
 const asset=readFileSync(new URL('./templates/asset.html',import.meta.url),'utf8');
 // The DOM contract, not a copied attribute spelling: the template writes the
 // data-system-code attribute and the browser exposes it as dataset.systemCode.
 assert.match(asset,/data-system-code="\{\{\.SystemCode\}\}"/);
 assert.match(js,/dataset\.systemCode/);
 assert.match(js,/trade_in_source: 'source', trade_in_destination: 'destination'/);
 assert.match(js,/select\.dataset\.tradeInBase/);
 assert.match(js,/window\.location\.assign\(`\$\{base\}\?direction=\$\{direction\}`\)/);
 assert.match(js,/if \(eventType && openTradeInForm\(eventType\)\) return;/);
 // The ordinary amount requirement and pattern rules stay untouched.
 assert.match(js,/amount\.required = true/);
 assert.match(js,/amount\.pattern = amount\.dataset\.positivePattern/);
});

test('FX evidence belongs to its own money group and is disclosed only for a foreign currency',()=>{
 assert.equal(tradeInFXVisible('cny','CNY'),false);
 assert.equal(tradeInFXVisible('USD','CNY'),true);
 assert.equal(tradeInFXVisible('','CNY'),false);
 assert.equal(tradeInFXVisible('CNY',''),false);
 const js=readFileSync(new URL('./static/trade-in.js',import.meta.url),'utf8');
 assert.match(js,/group\.querySelectorAll\('\[data-trade-in-fx\]'\)/);
 assert.doesNotMatch(js,/document\.querySelectorAll\('\[data-trade-in-fx\]'\)/);
});

test('the progressive money groups are initialized from the rendered currency values',async()=>{
 const fxField=()=>({hidden:false});
 const group=(currency,fields)=>({
  dataset:{},
  querySelector:(selector)=>selector==='[data-trade-in-currency]'
   ?{value:currency,dataset:{tradeInBase:'CNY'},addEventListener:()=>{}}
   :null,
  querySelectorAll:(selector)=>selector==='[data-trade-in-fx]'?fields:[],
 });
 const baseFX=[fxField(),fxField()],foreignFX=[fxField(),fxField(),fxField()];
 const groups=[group('CNY',baseFX),group('USD',foreignFX)];
 globalThis.document={querySelectorAll:(selector)=>selector==='[data-trade-in-money]'?groups:[],addEventListener:()=>{}};
 await import('./static/trade-in.js');
 assert.deepEqual(baseFX.map((field)=>field.hidden),[true,true]);
 assert.deepEqual(foreignFX.map((field)=>field.hidden),[false,false,false]);
});

test('the form works natively without JavaScript: separate search, review and final POST steps',()=>{
 const html=readFileSync(new URL('./templates/trade_in.html',import.meta.url),'utf8');
 assert.match(html,/action="\/assets\/\{\{\.Asset\.ID\}\}\/trade-in\/preview" method="post"/);
 assert.match(html,/name="step" value="search"/);
 assert.match(html,/name="step" value="review"/);
 assert.match(html,/name="goto" value="\{\{\$t\.PreviousPage\}\}"/);
 assert.match(html,/name="selected" value="\{\{\.AssetID\}\}"/);
 // The rendered page submits its own displayed rows: the selection merger is
 // decided by what the reviewer saw, not by a re-run of a changed query.
 assert.match(html,/name="displayed" value="\{\{\.AssetID\}\}"/);
 assert.match(html,/name="retain" value="\{\{\.\}\}"/);
 assert.match(html,/action="\/assets\/\{\{\.Asset\.ID\}\}\/trade-in" method="post"/);
 assert.match(html,/name="counterpart_id" value="\{\{\.\}\}"/);
 assert.match(html,/name="request_key" value="\{\{\$t\.RequestKey\}\}"/);
 // The FX evidence controls are server-rendered reachable and hidden by the
 // script for the base currency, so no-JavaScript users keep the fields.
 assert.doesNotMatch(html,/<label[^>]*data-trade-in-fx[^>]*hidden/);
 assert.match(html,/validation\.trade_in_event_stale/);
 // Trade-in money groups must not reuse the ordinary form-scoped FX hooks.
 assert.doesNotMatch(html,/data-fx-field/);
 assert.doesNotMatch(html,/data-currency-select/);
});

test('the asset page offers a native trade-in entry beside the enhanced selector and dedicated relation actions',()=>{
 const html=readFileSync(new URL('./templates/asset.html',import.meta.url),'utf8');
 assert.match(html,/href="\/assets\/\{\{\.Asset\.ID\}\}\/trade-in\?direction=source"/);
 assert.match(html,/href="\/assets\/\{\{\.Asset\.ID\}\}\/trade-in\?direction=destination"/);
 assert.match(html,/data-trade-in-base="\/assets\/\{\{\.Asset\.ID\}\}\/trade-in"/);
 assert.match(html,/data-system-code="\{\{\.SystemCode\}\}"/);
 assert.match(html,/trade_in\.no_money/);
 assert.match(html,/trade_in\.edit_link/);
 assert.match(html,/trade_in\.cancel_link/);
 // The relation anchor and the expand trigger stay siblings, never nested.
 const row=html.slice(html.indexOf('timeline-trigger'),html.indexOf('timeline-details'));
 const trigger=row.slice(0,row.indexOf('</button>'));
 assert.doesNotMatch(trigger,/<a href=/);
 assert.match(row.slice(row.indexOf('</button>')),/<p class="timeline-relation">[\s\S]*?<a href="\/assets\//);
});
