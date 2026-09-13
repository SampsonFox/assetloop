import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

test('asset drawers stack preview above usable fields independent of viewport width',()=>{
  const css=readFileSync(new URL('./static/app.css',import.meta.url),'utf8');
  assert.match(css,/dialog \.asset-profile\s*\{[^}]*grid-template-columns:minmax\(0,1fr\)/);
  assert.match(css,/dialog \.asset-product-visual\s*\{[^}]*min-height:0[^}]*height:180px/);
  assert.match(css,/dialog \.asset-profile-content\s*\{[^}]*padding:24px 0 0/);
  assert.match(css,/dialog \.asset-editor-fields\s*\{[^}]*repeat\(auto-fit,minmax\(min\(100%,220px\),1fr\)\)/);
  assert.match(css,/dialog\[data-drawer-kind="asset-editor"\] > \.drawer-panel\s*\{[^}]*width:min\(720px,100%\)/);
});

test('asset detail uses a compact custom-name or model heading without changing other pages',()=>{
  const template=readFileSync(new URL('./templates/asset.html',import.meta.url),'utf8');
  const header=template.split('<section class="card asset-profile">')[0];
  assert.ok(header.includes('asset-detail-heading'));
  assert.ok(header.includes('<h1>{{assetTitle .Asset}}</h1>'));
  assert.ok(!header.includes('assets.concrete'));
  assert.ok(header.includes('aria-label="{{t $s "assets.edit"}}"'));
  const css=readFileSync(new URL('./static/app.css',import.meta.url),'utf8');
  assert.match(css,/\.shell:has\(> \.asset-detail-heading\)\s*\{[^}]*padding-top:24px/);
  assert.match(css,/\.asset-detail-heading h1\s*\{[^}]*font-size:24px/);
  assert.match(css,/\.asset-detail-heading \.icon-button\s*\{[^}]*width:34px/);
  assert.match(css,/pointer:coarse[^]*\.asset-detail-heading \.icon-button\s*\{[^}]*width:44px/);
});
