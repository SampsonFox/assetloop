import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

test('asset detail uses a compact alias-only heading without changing other pages',()=>{
  const template=readFileSync(new URL('./templates/asset.html',import.meta.url),'utf8');
  const header=template.split('<section class="card asset-profile">')[0];
  assert.ok(header.includes('asset-detail-heading'));
  assert.ok(header.includes('<h1>{{.Asset.DisplayName}}</h1>'));
  assert.ok(!header.includes('assets.concrete'));
  assert.ok(header.includes('aria-label="{{t $s "assets.edit"}}"'));
  const css=readFileSync(new URL('./static/app.css',import.meta.url),'utf8');
  assert.match(css,/\.shell:has\(> \.asset-detail-heading\)\s*\{[^}]*padding-top:24px/);
  assert.match(css,/\.asset-detail-heading h1\s*\{[^}]*font-size:24px/);
  assert.match(css,/\.asset-detail-heading \.icon-button\s*\{[^}]*width:34px/);
  assert.match(css,/pointer:coarse[^]*\.asset-detail-heading \.icon-button\s*\{[^}]*width:44px/);
});
