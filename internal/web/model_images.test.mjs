import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');

test('model image uses stored URLs on detail, editor and list', () => {
  for (const path of ['templates/asset.html', 'templates/asset_form.html', 'templates/assets.html']) {
    const html = read(path);
    assert.match(html, /ImageURLs/);
    assert.doesNotMatch(html, /product-demo-iphone/);
  }
});
test('image caption occupies its own layout space instead of covering the product', () => {
  const css = read('static/app.css');
  const caption = css.match(/\.asset-product-caption\s*\{([^}]+)\}/)[1];
  assert.match(caption, /position:static/);
  assert.doesNotMatch(caption, /position:absolute/);
});
test('model image configuration remains separate from 3D and uses accessible icon actions', () => {
  const html = read('templates/model_image.html');
  assert.match(html, /multipart\/form-data/);
  assert.match(html, /image\/png,image\/jpeg,image\/webp/);
  assert.match(html, /aria-label="{{t \$s "image.clear"}}"/);
  assert.match(html, /name="action" value="import"/);
  assert.match(read('templates/catalog_drawers.html'), /data-model-image-link/);
  assert.match(read('static/app.js'), /image\.hidden = !modelId/);
});
