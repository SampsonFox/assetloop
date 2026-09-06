import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');

test('tag management has compact actions and uses its own table styling', () => {
  const html = read('./templates/specifications.html');
  assert.match(html, /class="heading-actions"><a class="button auto"/);
  assert.match(html, /class="catalog-table specification-table"/);
  assert.match(html, /class="specification-navigation"/);
  assert.match(read('./static/app.css'), /\.specification-table th:last-child[^}]*white-space:nowrap/);
});
test('item tag dimensions use the themed fieldset styling', () => {
  assert.match(read('./static/app.css'), /\.asset-tag-dimension[^}]*border:1px solid var\(--line\)/);
});
test('appearance candidate and manual rows retain mobile field labels', () => {
  const html = read('./templates/appearance.html');
  assert.equal((html.match(/<td data-label="{{t \$s "resource.name"}}"/g) || []).length, 2);
  assert.equal((html.match(/<td data-label="{{t \$s "assets.status"}}"/g) || []).length, 2);
});
