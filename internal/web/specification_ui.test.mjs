import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');

test('tag management has compact actions and uses its own table styling', () => {
  const html = read('./templates/specifications.html');
  assert.match(html, /class="heading-actions"><a class="icon-button management-primary"/);
  assert.match(html, /aria-label="{{if \$data.Values}}{{t \$s "tags.add_value"}}/);
  assert.match(html, /{{template "ui-icon" "plus"}}/);
  assert.match(html, /class="catalog-table specification-table"/);
  assert.match(html, /class="specification-navigation"/);
  assert.match(read('./static/app.css'), /\.specification-table th:last-child[^}]*white-space:nowrap/);
});
test('item tag dimensions use the themed fieldset styling', () => {
  assert.match(read('./static/app.css'), /\.asset-tag-dimension[^}]*border:1px solid var\(--line\)/);
});
test('drawer panels own bounded scrolling rather than trapping outer-dialog scroll', () => {
  const css = read('./static/app.css');
  const panel = css.match(/^\.drawer-panel \{([^}]+)\}/m)?.[1] || '';
  const dialog = css.match(/^dialog\.drawer \{([^}]+)\}/m)?.[1] || '';
  assert.match(panel, /(?:^|;)\s*height:100%;/, 'panel height must be bounded by the viewport dialog');
  assert.match(panel, /overflow:auto/);
  assert.match(dialog, /overflow:hidden/, 'outer dialog must not compete for wheel input');
  assert.match(css, /\.event-drawer-panel \{[^}]*overflow:hidden/);
  assert.match(css, /\.drawer-body \{[^}]*min-height:0;[^}]*overflow:auto/);
});
test('dictionary active navigation and heading use valid restrained theme styles', () => {
  const css = read('./static/app.css');
  assert.match(css, /\.specification-navigation a\[aria-current="page"\][^}]*color:var\(--ink\)/);
  assert.match(css, /\.specification-navigation a\[aria-current="page"\][^}]*text-decoration-line:underline/);
  assert.match(css, /\.specification-heading h1[^}]*letter-spacing:-\.03em/);
  assert.match(read('./templates/specifications.html'), /class="page-heading specification-heading"/);
});
test('appearance candidate and manual rows retain mobile field labels', () => {
  const html = read('./templates/appearance.html');
  assert.equal((html.match(/<td data-label="{{t \$s "resource.name"}}"/g) || []).length, 2);
  assert.equal((html.match(/<td data-label="{{t \$s "assets.status"}}"/g) || []).length, 2);
});
