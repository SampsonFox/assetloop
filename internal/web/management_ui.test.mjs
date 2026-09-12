import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');

test('drawer actions have breathing room and the shell requests the updated stylesheet', () => {
  assert.match(read('./static/app.css'), /\.drawer-heading-actions \{[^}]*gap:12px/);
  assert.match(read('./templates/base.html'), /app\.css\?v=market-discovery-2/);
});

test('appearance reset has separation from transfer panels', () => {
  assert.match(read('./static/tag-transfer.js'), /field-heading transfer-heading/);
  assert.match(read('./static/app.css'), /\.transfer-heading \{[^}]*margin-bottom:16px/);
});
test('icon actions retain accessible names and tooltips while tabs retain text', () => {
  for (const name of ['specifications', 'resources', 'resource', 'event_types', 'catalog_drawers', 'appearance', 'market']) {
    const html = read(`./templates/${name}.html`);
    const actions = [...html.matchAll(/<(?:a|button)\b([^>]+)>{{template "ui-icon" "[^"]+"}}/g)];
    assert.ok(actions.length >= 2, name);
    for (const [, attrs] of actions) {
      assert.match(attrs, /aria-label="/);
      assert.match(attrs, /title="/);
    }
  }
  assert.match(read('./templates/specifications.html'), />{{t \$s "tags.values"}}<\/a>/);
  assert.match(read('./templates/specifications.html'), />{{t \$s "tags.types"}}<\/a>/);
});
test('management tables share compact actions and drawer footers share alignment', () => {
  const css = read('./static/app.css');
  assert.match(css, /\.catalog-table td \.icon-button,\.resource-table td \.icon-button,\.appearance-rules \.icon-button,\s*\.event-type-actions \.icon-button \{[^}]*width:30px/);
  assert.match(css, /\.drawer-footer \{ justify-content:flex-end/);
  for (const name of ['catalog_drawers', 'appearance']) {
    // Management drawers deliberately use text actions in their fixed heading.
    const html = read(`./templates/${name}.html`).replace(/<div class="drawer-heading-actions">[\s\S]*?<\/div>/g, '');
    assert.doesNotMatch(html, /<button[^>]*>{{t \$s "(?:common.save|common.cancel|appearance.confirm|tags.save_allowed)"}}/);
  }
  for (const name of ['catalog_drawers', 'specifications', 'resources', 'event_types']) {
    const drawers = [...read(`./templates/${name}.html`).matchAll(/<dialog\b[^>]*>([\s\S]*?)<\/dialog>/g)];
    assert.ok(drawers.length, name);
    for (const [whole, body] of drawers) {
      assert.match(whole, /data-management-drawer/);
      const header = body.split('</header>')[0];
      if (!body.includes('<form')) {
        assert.match(header, /data-dialog-close/);
        assert.doesNotMatch(body, /type="submit"/);
        continue; // Immutable built-ins and Viewer details have no save action.
      }
      assert.match(header, /class="drawer-heading-actions"/);
      assert.match(header, /data-dialog-close>{{t \$s "common.cancel"}}<\/button>(?:{{if \.CanManageCatalog}})?<button[^>]*type="submit"/);
      assert.equal((body.match(/data-dialog-close/g) || []).length, 1);
      assert.equal((body.replace(/<noscript>[\s\S]*?<\/noscript>/g, '').match(/type="submit"/g) || []).length, 1);
      assert.doesNotMatch(body, /<footer|eyebrow/);
      assert.match(body, /(?:model-drawer-body|drawer-body stack)/);
      assert.match(body, /data-guard-dirty/);
      assert.match(body, /name="csrf_token"/);
    }
  }
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
test('built-in types have a noninteractive lock and compact row actions', () => {
 const html = read('./templates/event_types.html');
 assert.match(html, /else if .BuiltIn}}<span class="event-type-lock" role="img"/);
 assert.match(html, /aria-label="{{t \$s "validation.event_type_builtin"}}"/);
 const css = read('./static/app.css');
 assert.match(css, /\.event-type-actions \.icon-button \{[^}]*width:30px;[^}]*height:30px/);
 assert.match(css, /@media \(pointer:coarse\)[^{]*\{[^}]*event-type-actions \.icon-button[^}]*44px/);
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

test("market discovery preserves icon labels, explicit scope and the refreshed list filter",()=>{const html=read("./templates/market.html");assert.match(html,/name="accept_scope" value="1" required/);assert.match(html,/name="candidate"/);assert.match(html,/market.product_scope_help/);assert.match(read("./static/drawer-stack.js"),/:scope > \.management-filters/);assert.doesNotMatch(html,/<img/);});

test("market controls do not shadow native form routing properties",()=>{assert.doesNotMatch(read("./templates/market.html"),/name="(?:action|method|submit)"/);});
