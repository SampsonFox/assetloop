import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');

test('appearance reset has separation from transfer panels', () => {
  assert.match(read('./static/tag-transfer.js'), /field-heading transfer-heading/);
  assert.match(read('./static/app.css'), /\.transfer-heading \{[^}]*margin-bottom:16px/);
});
test('icon actions retain accessible names and tooltips while tabs retain text', () => {
  for (const name of ['specifications', 'resources', 'resource', 'event_types']) {
    const html = read(`./templates/${name}.html`);
    const actions = [...html.matchAll(/<(?:a|button)\b([^>]+)>{{template "ui-icon" "[^"]+"}}/g)];
    assert.ok(actions.length >= 3);
    for (const [, attrs] of actions) {
      assert.match(attrs, /aria-label="/);
      assert.match(attrs, /title="/);
    }
  }
  assert.match(read('./templates/specifications.html'), />{{t \$s "tags.values"}}<\/a>/);
  assert.match(read('./templates/specifications.html'), />{{t \$s "tags.types"}}<\/a>/);
});
test('event type actions retain status confirmation and management permissions', () => {
 const html = read('./templates/event_types.html');
 assert.match(html, /asset-filters management-filters/);
 assert.match(html, /class="event-type-actions"/);
 assert.match(html, /and \$\.CanManageLifecycle \(not .BuiltIn\)/);
 assert.match(html, /data-confirm="{{t \$s "types.stop_confirm"}}"/);
 assert.match(html, /aria-label="{{if .Enabled}}{{t \$s "types.stop"}}{{else}}{{t \$s "types.restore"}}/);
 assert.match(read('./static/app.css'), /\.event-type-actions[^}]*gap:8px;[^}]*white-space:nowrap/);
});
test('resource layout combines file metadata and preserves full attribution on demand', () => {
  const html = read('./templates/resources.html');
  assert.match(html, /GLB · {{resourceSize .SizeBytes}}/);
  assert.match(html, /<details class="resource-license"><summary>{{resourceLicenseSummary .License}}<\/summary><p>{{.License}}<\/p>/);
  assert.match(html, /<div class="resource-actions">/);
  assert.doesNotMatch(html, /<td[^>]*class="row-actions"/);
  assert.match(read('./static/app.css'), /resource-name-col \{ width:36%/);
  assert.match(read('./static/app.css'), /@media \(max-width:700px\)/);
});
