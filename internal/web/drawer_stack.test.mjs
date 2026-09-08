import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const source = readFileSync(new URL('./static/drawer-stack.js', import.meta.url), 'utf8');
const origin = 'http://localhost';
test('drawer exits with a short fade and waits for animation rather than a fixed delay', () => {
  const css = readFileSync(new URL('./static/app.css', import.meta.url), 'utf8');
  assert.match(css, /transition:transform 200ms cubic-bezier\(\.25,\.46,\.45,\.94\)/);
  assert.match(css, /@starting-style[^\n]+translateX\(20px\)/);
  assert.match(css, /data-stack-closing[^\n]+translateX\(8px\)[^\n]+opacity:0/);
  assert.match(css, /data-stack-returning[^\n]+transition-duration:120ms/);
  assert.match(css, /html\s*\{\s*scrollbar-gutter:stable/);
  assert.match(css, /prefers-reduced-motion:reduce[^\n]+transition:none/);
  assert.match(source, /getAnimations/);
  assert.doesNotMatch(source, /setTimeout\(resolve,200\)/);
  const app = readFileSync(new URL('./static/app.js', import.meta.url), 'utf8');
  assert.match(app, /await window\.assetloopDrawers\?\.beforeClose\(dialog\);\s*dialog\.close\(\);\s*discardDialogForms\(dialog\)/);
});
const dataKey = name => name.slice(5).replace(/-([a-z])/g, (_, c) => c.toUpperCase());

// Only browser boundaries are faked: the complete production IIFE runs unchanged.
class Element {
  constructor(tag) {
    this.tagName = tag.toUpperCase(); this.attributes = new Map(); this.dataset = {};
    this.childNodes = []; this.listeners = new Map(); this.parentNode = null;
    this.style = {setProperty(name, value) { this[name] = value; }};
    this.value = ''; this.disabled = false; this.open = false; this.text = '';
  }
  setAttribute(name, value) {
    if (name.startsWith('data-')) this.dataset[dataKey(name)] = String(value);
    else this.attributes.set(name, String(value));
  }
  getAttribute(name) { return name.startsWith('data-') ? this.dataset[dataKey(name)] ?? null : this.attributes.get(name) ?? null; }
  hasAttribute(name) { return name === 'open' ? this.open : this.getAttribute(name) !== null; }
  removeAttribute(name) { if (name.startsWith('data-')) delete this.dataset[dataKey(name)]; else this.attributes.delete(name); }
  toggleAttribute(name, force) { if (force ?? !this.hasAttribute(name)) this.setAttribute(name, ''); else this.removeAttribute(name); }
  get id() { return this.getAttribute('id') || ''; } set id(v) { this.setAttribute('id', v); }
  get className() { return this.getAttribute('class') || ''; } set className(v) { this.setAttribute('class', v); }
  get href() { return new URL(this.getAttribute('href') || '', origin).href; } set href(v) { this.setAttribute('href', v); }
  get action() { return new URL(this.getAttribute('action') || '', origin).href; } set action(v) { this.setAttribute('action', v); }
  get method() { return this.getAttribute('method') || 'get'; } set method(v) { this.setAttribute('method', v); }
  get name() { return this.getAttribute('name') || ''; } set name(v) { this.setAttribute('name', v); }
  get type() { return this.getAttribute('type') || ''; } set type(v) { this.setAttribute('type', v); }
  get hidden() { return this.hasAttribute('hidden'); } set hidden(v) { this.toggleAttribute('hidden', v); }
  get children() { return this.childNodes; }
  get isConnected() { return this.tagName === 'BODY' || !!this.parentNode?.isConnected; }
  get textContent() { return this.text + this.childNodes.map(n => n.textContent).join(''); }
  set textContent(value) { this.replaceChildren(); this.text = String(value); }
  get options() { return this.querySelectorAll('option'); }
  get selectedOptions() { return this.options.filter(option => option.value === this.value); }
  cloneNode(deep = false) { const node = new Element(this.tagName); node.attributes = new Map(this.attributes); node.dataset = {...this.dataset}; node.value = this.value; node.text = this.text; node.disabled = this.disabled; if (deep) node.append(...this.childNodes.map(child => child.cloneNode(true))); return node; }
  add(node) { this.append(node); }
  append(...nodes) { for (const node of nodes) { node.remove(); node.parentNode = this; this.childNodes.push(node); } }
  prepend(node) { node.remove(); node.parentNode = this; this.childNodes.unshift(node); }
  remove() { if (this.parentNode) this.parentNode.childNodes = this.parentNode.childNodes.filter(n => n !== this); this.parentNode = null; }
  replaceWith(node) { const parent = this.parentNode, index = parent.childNodes.indexOf(this); node.remove(); parent.childNodes[index] = node; node.parentNode = parent; this.parentNode = null; }
  replaceChildren(...nodes) { for (const child of [...this.childNodes]) child.remove(); this.text = ''; this.append(...nodes); }
  matches(selector) {
    return selector.split(',').some(part => {
      let s = part.trim();
      const not = [...s.matchAll(/:not\(([^)]+)\)/g)];
      if (not.some(([, inner]) => this.matches(inner))) return false;
      s = s.replace(/:not\([^)]+\)/g, '');
      for (const [, name, operator, quoted, bare] of s.matchAll(/\[([^\s=\]^]+)(\^?=)?(?:"([^"]*)"|([^\]]*))?\]/g)) {
        if (!this.hasAttribute(name)) return false;
        const expected = quoted ?? bare;
        if (operator === '=' && this.getAttribute(name) !== expected) return false;
        if (operator === '^=' && !this.getAttribute(name).startsWith(expected)) return false;
      }
      s = s.replace(/\[[^\]]*\]/g, '');
      const tag = s.match(/^[\w-]+/)?.[0];
      if (tag && tag.toUpperCase() !== this.tagName) return false;
      return [...s.matchAll(/\.([\w-]+)/g)].every(([, c]) => this.className.split(' ').includes(c));
    });
  }
  querySelectorAll(selector) { return this.childNodes.flatMap(child => [...(child.matches(selector) ? [child] : []), ...child.querySelectorAll(selector)]); }
  querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
  closest(selector) { return this.matches(selector) ? this : this.parentNode?.closest(selector) || null; }
  addEventListener(name, fn) { if (!this.listeners.has(name)) this.listeners.set(name, []); this.listeners.get(name).push(fn); }
  dispatchEvent(event) { event.target ??= this; for (const fn of this.listeners.get(event.type) || []) fn(event); if (event.bubbles) this.parentNode?.dispatchEvent(event); return true; }
  showModal() { this.open = true; }
  close() { this.open = false; this.dispatchEvent({type: 'close'}); }
  focus() { this.focused = true; }
  getBoundingClientRect() { return {width: 640}; }
}

const el = (tag, attrs = {}, children = []) => {
  const node = new Element(tag);
  for (const [name, value] of Object.entries(attrs)) node.setAttribute(name, value);
  node.append(...children); return node;
};
function fragment() {
  const input = el('input', {id: 'name', name: 'name', 'aria-describedby': 'help external-help'}); input.value = 'server value';
  return el('dialog', {id: 'tag-editor', class: 'drawer', 'aria-labelledby': 'title'}, [
    el('div', {class: 'drawer-panel'}, [
      el('h2', {id: 'title'}), el('p', {id: 'help'}),
      el('button', {form: 'editor-form', 'aria-controls': 'name help'}),
      el('form', {id: 'editor-form', action: '/admin/tags/values/value-1', method: 'post'}, [
        el('label', {for: 'name'}), input, el('a', {href: '#help'}),
      ]), el('section', {'data-drawer-query-results': 'candidates'}), el('script'),
    ]),
  ]);
}
function harness({reducedMotion=true}={}) {
  const body = el('body'), requests = [], closeCalls = [], initializations = [], observers = [];
  const document = el('document'); document.body = body; document.documentElement = {lang: 'en'};
  document.getElementById = () => null; // This harness has no settings root to refresh.
  document.querySelectorAll = s => body.querySelectorAll(s);
  document.createElement = tag => el(tag); document.createTextNode = text => { const n = el('text'); n.textContent = text; return n; };
  const pages = new Map(); let pageID = 0, permitClose = true;
  const window = {addEventListener() {}, assetloopDialog: {
    initialize(dialog) { initializations.push(dialog); },
    async close(dialog) { closeCalls.push(dialog); if (!permitClose) return false; await window.assetloopDrawers.beforeClose(dialog); dialog.close(); return true; },
  }};
  const context = {window, document, location: {href: origin + '/admin/catalog', origin}, innerWidth: 1200,
    URL, URLSearchParams, AbortController, setTimeout,
    matchMedia: () => ({matches: reducedMotion}),
    Event: class { constructor(type, opts = {}) { Object.assign(this, {type}, opts); } },
    CustomEvent: class { constructor(type, opts = {}) { Object.assign(this, {type}, opts); } },
    MutationObserver: class { constructor(fn) { observers.push(fn); } observe() {} },
    DOMParser: class { parseFromString(key) { assert.ok(pages.has(key), 'response fixture must be registered'); return pages.get(key); } },
    Option: function(text, value) { const option = el('option'); option.textContent = text; option.value = value; return option; },
    FormData: class extends URLSearchParams { constructor(form) { super(); for (const n of form.querySelectorAll('[name]')) if (!n.disabled) this.append(n.name, n.value); } },
    fetch(url, options) { return new Promise((resolve, reject) => requests.push({url, options, resolve, reject})); },
  };
  vm.runInNewContext(source, context, {filename: 'drawer-stack.js'});
  function respond(index, {status = 200, tree = fragment(), json, readonly = false} = {}) {
    const key = 'fixture-' + ++pageID; const page = el('document', {}, [tree]); pages.set(key, page);
    requests[index].resolve({status, ok: status >= 200 && status < 300, url: requests[index].url,
      headers: {get: name => name === 'X-Assetloop-Readonly' ? String(readonly) : name === 'Content-Type' ? (json ? 'application/json' : 'text/html') : null}, text: async () => key, json: async () => json});
  }
  function native(id = 'model-drawer', action = '/admin/catalog/models/model-1') {
    const form = el('form', {action, method: 'post'}, [el('input', {name: 'name'})]);
    const dialog = el('dialog', {id, class: 'drawer'}, [el('div', {class: 'drawer-panel'}, [el('button'), form])]);
    body.append(dialog); dialog.showModal(); window.assetloopDrawers.opened(dialog); return dialog;
  }
  function link(parent, id = 'value-1', target = 'tag-editor') {
    const node = el('a', {href: '/admin/tags?edit=' + id, 'data-drawer-target': target}); parent.querySelector('.drawer-panel').append(node); return node;
  }
  return {body, document, requests, respond, native, link, closeCalls, initializations,
    click(target, options = {}) { const event = {type: 'click', target, button: 0, defaultPrevented: false, stopped: false,
      preventDefault() { this.defaultPrevented = true; }, stopImmediatePropagation() { this.stopped = true; }, ...options}; document.dispatchEvent(event); return event; },
    api: window.assetloopDrawers, close: window.assetloopDialog.close,
    permitClose(value) { permitClose = value; },
    remote() { return body.querySelectorAll('dialog').at(-1); },
  };
}
const settle = () => new Promise(resolve => setImmediate(resolve));
test('child drawer width belongs to its target rather than its parent',async()=>{
 const h=harness(),parent=h.native('resource-editor','/admin/3d/resource-1');
 parent.querySelector('.drawer-panel').style.width='920px';
 const pending=h.api.open(h.link(parent,'value-1','tag-editor'));
 assert.equal(h.remote().querySelector('.drawer-panel').style.width,undefined);
 assert.equal(h.remote().dataset.drawerKind,'tag-editor');
 h.respond(0);await pending;
 assert.equal(h.remote().querySelector('.drawer-panel').style.width,undefined);
 assert.equal(parent.querySelector('.drawer-panel').style.width,'920px');
});
test('nested closing retains exit state until hidden and synchronizes all returning parents', async () => {
  const h=harness(), parent=h.native(), child=await loaded(h,parent,'nested');
  await h.api.beforeClose(child);
  assert.equal(child.open,true);
  assert.equal(child.hasAttribute('data-stack-closing'),true);
  assert.equal(parent.hasAttribute('data-stack-returning'),true);
  child.close();
  assert.equal(child.hasAttribute('data-stack-closing'),false);
  assert.equal(parent.hasAttribute('data-stack-returning'),false);
  await h.api.beforeClose(parent);
  assert.equal(parent.hasAttribute('data-stack-closing'),true);
  parent.close();
  assert.equal(parent.hasAttribute('data-stack-closing'),false);
});
test('closing finishes with the panel animation, including cancellation and no-animation cases', async () => {
  const h=harness({reducedMotion:false}), dialog=h.native();
  let finish;
  dialog.querySelector('.drawer-panel').getAnimations=()=>[{finished:new Promise(resolve=>{finish=resolve;})}];
  const pending=h.close(dialog);
  await settle(); assert.equal(dialog.open,true);
  finish(); await pending; assert.equal(dialog.open,false);
  const cancelled=h.native();
  cancelled.querySelector('.drawer-panel').getAnimations=()=>[{finished:Promise.reject(new Error('cancelled'))}];
  await h.close(cancelled); assert.equal(cancelled.open,false);
  const still=h.native(); still.querySelector('.drawer-panel').getAnimations=()=>[];
  await h.close(still); assert.equal(still.open,false);
});
async function loaded(h, parent, id) {
  const pending = h.api.open(h.link(parent, id)); const index = h.requests.length - 1;
  h.respond(index); await pending; return h.remote();
}
function submit(h, form) { let prevented = false; const handled = h.api.submit({target: form, preventDefault() { prevented = true; }}); assert.equal(handled, true); assert.equal(prevented, true); }

test('loaded fragments namespace IDs, labels, ARIA, form owners and local anchors independently', async () => {
  const h = harness(), parent = h.native(), parentForm = parent.querySelector('form'), draft = parent.querySelector('input');
  draft.value = 'parent draft';
  const first = await loaded(h, parent, 'one'), second = await loaded(h, first, 'two');
  assert.equal(parent.querySelector('form'), parentForm); assert.equal(parent.querySelector('input'), draft); assert.equal(draft.value, 'parent draft');
  const seen = new Set();
  for (const dialog of [first, second]) {
    const input = dialog.querySelector('input'), form = dialog.querySelector('form'), heading = dialog.querySelector('h2'), help = dialog.querySelector('p');
    for (const node of dialog.querySelectorAll('[id]')) { assert.ok(!seen.has(node.id)); seen.add(node.id); assert.notEqual(node.id, 'name'); }
    assert.equal(dialog.getAttribute('aria-labelledby'), heading.id);
    assert.equal(dialog.querySelector('label').getAttribute('for'), input.id);
    assert.equal(input.getAttribute('aria-describedby'), help.id + ' external-help');
    assert.equal(dialog.querySelector('button').getAttribute('form'), form.id);
    assert.equal(dialog.querySelector('button').getAttribute('aria-controls'), input.id + ' ' + help.id);
    assert.equal(dialog.querySelector('a').getAttribute('href'), '#' + help.id);
    assert.equal(dialog.querySelector('script'), null);
  }
});

test('cancel aborts a child load and late responses cannot replace or reopen parent DOM', async () => {
  const h = harness(), parent = h.native(), panel = parent.querySelector('.drawer-panel'), input = parent.querySelector('input');
  input.value = 'parent draft'; const pending = h.api.open(h.link(parent)); const child = h.remote();
  assert.equal(parent.hasAttribute('data-covered'), true);
  await h.close(child); assert.equal(h.requests[0].options.signal.aborted, true);
  h.respond(0); await pending;
  assert.equal(child.isConnected, false); assert.equal(parent.open, true);
  assert.equal(parent.querySelector('.drawer-panel'), panel); assert.equal(parent.querySelector('input'), input);
  assert.equal(input.value, 'parent draft'); assert.equal(parent.hasAttribute('data-covered'), false);
});

test('a newer GET wins over an earlier response even if the aborted request resolves', async () => {
  const h = harness(), parent = h.native(), child = await loaded(h, parent);
  const form = child.querySelector('form'); form.method = 'get';
  form.querySelector('input').value = 'first'; submit(h, form);
  form.querySelector('input').value = 'second'; submit(h, form);
  assert.equal(h.requests[1].options.signal.aborted, true);
  const originalInput = form.querySelector('input');
  const latest = fragment(); latest.querySelector('[data-drawer-query-results]').textContent = 'latest'; h.respond(2, {tree: latest}); await settle();
  const stale = fragment(); stale.querySelector('[data-drawer-query-results]').textContent = 'stale'; h.respond(1, {tree: stale}); await settle();
  assert.equal(child.querySelector('[data-drawer-query-results]').textContent, 'latest');
  assert.equal(child.querySelector('form'), form); assert.equal(form.querySelector('input'), originalInput);
  assert.equal(originalInput.value, 'second', 'query refresh preserves the live form draft');
});

test('duplicate POST is suppressed; validation and network failures retain the same draft nodes', async () => {
  const h = harness(), parent = h.native(), child = await loaded(h, parent), form = child.querySelector('form'), input = form.querySelector('input');
  input.value = 'unsaved draft'; const button = child.querySelector('button');
  const disabledButton = el('button'); disabledButton.disabled = true; child.querySelector('.drawer-panel').append(disabledButton);
  submit(h, form); submit(h, form);
  assert.equal(h.requests.length, 2); assert.equal(h.requests[1].options.method, 'POST'); assert.equal(button.disabled, true);
  assert.equal(h.requests[1].options.headers['X-Assetloop-Drawer'], 'tag-editor');
  assert.equal(h.requests[1].options.body.get('name'), 'unsaved draft');
  const error = el('p', {'data-error-summary': ''}); error.textContent = 'Name already exists';
  h.respond(1, {status: 422, tree: error}); await settle();
  assert.equal(child.querySelector('form'), form); assert.equal(form.querySelector('input'), input); assert.equal(input.value, 'unsaved draft');
  assert.match(child.querySelector('[data-stack-feedback]').textContent, /Name already exists/);
  assert.equal(button.disabled, false); assert.notEqual(form.dataset.submitting, 'true');
  assert.equal(disabledButton.disabled, true, 'failure must preserve preexisting disabled state');
  submit(h, form); h.requests[2].reject(new Error('offline')); await settle();
  assert.equal(child.open, true); assert.equal(input.value, 'unsaved draft'); assert.equal(button.disabled, false);
  assert.match(child.querySelector('[data-stack-feedback]').textContent, /retry/i);
});

test('successful child POST closes only the child and synchronizes the parent select in place', async () => {
  const h = harness(), parent = h.native(), select = el('select', {name: 'category_id'}), input = parent.querySelector('input');
  const saved = []; h.document.addEventListener('drawer:saved', event => saved.push(event.detail));
  parent.querySelector('form').append(select); input.value = 'parent draft';
  const link = h.link(parent, '', 'category-drawer'); link.href = '/admin/catalog/categories/new';
  const pending = h.api.open(link); h.respond(0); await pending;
  const child = h.remote(), form = child.querySelector('form'); submit(h, form);
  h.respond(1, {json: {kind: 'category', id: 'new-category', name: 'New category', enabled: true}}); await settle();
  assert.equal(child.isConnected, false); assert.equal(parent.open, true); assert.equal(parent.querySelector('select'), select);
  assert.equal(select.value, 'new-category'); assert.equal(select.options[0].textContent, 'New category');
  assert.equal(input.value, 'parent draft'); assert.match(parent.querySelector('[data-stack-feedback]').textContent, /independently/);
  assert.equal(saved.length, 1); assert.equal(saved[0].parent, parent); assert.equal(saved[0].created, true); assert.equal(saved[0].id, 'new-category');
});

test('editing an existing entity updates its label without changing the parent selection', async () => {
  const h = harness(), parent = h.native(), select = el('select', {name: 'category_id'});
  const saved = []; h.document.addEventListener('drawer:saved', event => saved.push(event.detail));
  const chosen = el('option'), edited = el('option'); chosen.value = 'chosen'; edited.value = 'edited';
  chosen.textContent = 'Chosen'; edited.textContent = 'Old label'; select.append(chosen, edited); select.value = 'chosen'; parent.querySelector('form').append(select);
  const link = h.link(parent, 'edited', 'category-drawer'); link.href = '/admin/catalog/categories/edited';
  const pending = h.api.open(link); h.respond(0); await pending;
  const child = h.remote(); submit(h, child.querySelector('form'));
  h.respond(1, {json: {kind: 'category', id: 'edited', name: 'Updated label', enabled: true}}); await settle();
  assert.equal(select.value, 'chosen'); assert.equal(edited.textContent, 'Updated label'); assert.equal(select.options.length, 2);
  assert.equal(child.isConnected, false);
  assert.equal(saved.length, 1); assert.equal(saved[0].parent, parent); assert.equal(saved[0].created, false);
});

test('appearance identity distinguishes rules and models while repeated rules return to their ancestor', async () => {
  const h = harness(), parent = h.native();
  const openRule = async (owner, model, rule) => {
    const link = h.link(owner, '', 'appearance-editor'); link.href = `/admin/catalog/models/${model}/appearance?rule_id=${rule}`;
    const pending = h.api.open(link); h.respond(h.requests.length - 1); await pending; return h.remote();
  };
  const first = await openRule(parent, 'model-1', 'rule-a');
  const second = await openRule(first, 'model-1', 'rule-b');
  const third = await openRule(second, 'model-2', 'rule-a');
  assert.equal(h.requests.length, 3); assert.equal(first.open, true); assert.equal(second.open, true);
  const back = h.link(third, '', 'appearance-editor'); back.href = '/admin/catalog/models/model-1/appearance?rule_id=rule-a';
  await h.api.open(back);
  assert.equal(h.requests.length, 3); assert.deepEqual(h.closeCalls, [third, second]); assert.equal(first.open, true);
});

test('query responses without a results region fail locally and retain the editor', async () => {
  const h = harness(), parent = h.native(), child = await loaded(h, parent), form = child.querySelector('form');
  form.method = 'get'; form.querySelector('input').value = 'live query'; const oldResults = child.querySelector('[data-drawer-query-results]');
  submit(h, form); const missing = fragment(); missing.querySelector('[data-drawer-query-results]').remove();
  h.respond(1, {tree: missing}); await settle();
  assert.equal(child.querySelector('form'), form); assert.equal(form.querySelector('input').value, 'live query');
  assert.equal(child.querySelector('[data-drawer-query-results]'), oldResults);
  assert.match(child.querySelector('[data-stack-feedback]').textContent, /retry/i);
});

test('push layout follows opening order, not DOM order; ancestor return closes descendants in reverse order', async () => {
  const h = harness(), earlierDOM = h.native('category-drawer', '/admin/catalog/categories/category-1');
  earlierDOM.close();
  const parent = h.native(); earlierDOM.showModal(); h.api.opened(earlierDOM);
  const child = await loaded(h, earlierDOM);
  assert.equal(parent.querySelector('.drawer-panel').style['--drawer-push'], '128px');
  assert.equal(earlierDOM.querySelector('.drawer-panel').style['--drawer-push'], '64px');
  assert.equal(child.querySelector('.drawer-panel').style['--drawer-push'], '0px');
  const back = el('a', {href: '/admin/catalog?edit_model_id=model-1', 'data-drawer-target': 'model-drawer'}); child.querySelector('.drawer-panel').append(back);
  await h.api.open(back);
  assert.deepEqual(h.closeCalls, [child, earlierDOM]); assert.equal(h.requests.length, 1);
  assert.equal(parent.open, true); assert.equal(parent.querySelector('.drawer-panel').style['--drawer-push'], '0px');
});

test('rejected ancestor close keeps the complete stack and does not fetch a duplicate ancestor', async () => {
  const h = harness(), parent = h.native(), child = await loaded(h, parent);
  h.permitClose(false);
  const back = el('a', {href: '/admin/catalog?edit_model_id=model-1', 'data-drawer-target': 'model-drawer'}); child.append(back);
  await h.api.open(back);
  assert.equal(parent.open, true); assert.equal(child.open, true); assert.equal(h.requests.length, 1);
  assert.deepEqual(h.closeCalls, [child]);
});

test('visible root model buttons fetch SSR values instead of relying on opener name data', async () => {
  const h = harness(), native = h.native(); native.close();
  const button = el('button', {'data-dialog-open': 'model-drawer', 'data-edit-model-id': 'model-1'});
  const icon = el('svg'); button.append(icon); h.body.append(button);
  const event = h.click(icon);
  assert.equal(event.defaultPrevented, true); assert.equal(event.stopped, true);
  assert.equal(h.requests.length, 1);
  const url = new URL(h.requests[0].url);
  assert.equal(url.pathname, '/admin/catalog'); assert.equal(url.searchParams.get('edit_model_id'), 'model-1');
  assert.equal(h.requests[0].options.headers['X-Assetloop-Drawer'], 'model-drawer');
  assert.equal(h.requests[0].options.cache, 'no-store', 'details must not reuse an older full-page or drawer response');
  const tree = fragment(); tree.querySelector('input').value = 'Persisted model name';
  h.respond(0, {tree}); await settle();
  const remote = h.remote(); assert.notEqual(remote, native); assert.equal(native.open, false);
  assert.equal(remote.querySelector('input').value, 'Persisted model name');
  await h.close(remote); assert.equal(button.focused, true, 'closing returns focus to the actual root opener');
});

test('supported visible root create/edit buttons use their matching fragment routes', async () => {
  const cases = [
    ['model-drawer', {}, '/admin/catalog?dialog=model-drawer'],
    ['category-drawer', {}, '/admin/catalog/categories/new'],
    ['category-drawer', {'data-action': '/admin/catalog/categories/category-1'}, '/admin/catalog/categories/category-1'],
    ['resource-upload', {}, '/admin/3d?dialog=resource-upload'],
    ['event-type-manage', {}, '/admin/event-types?dialog=event-type-manage'],
  ];
  for (const [target, attrs, expected] of cases) {
    const h = harness(), button = el('button', {'data-dialog-open': target, ...attrs}); h.body.append(button);
    const event = h.click(button); assert.equal(event.stopped, true, target);
    assert.equal(new URL(h.requests[0].url, origin).href, origin + expected);
    assert.equal(h.requests[0].options.headers['X-Assetloop-Drawer'], target);
    h.respond(0); await settle(); assert.equal(h.remote().open, true);
  }
});

test('hidden automatic openers, unsupported dialogs and modified clicks retain their original path', () => {
  for (const target of ['model-drawer', 'category-drawer', 'resource-upload', 'event-type-manage']) {
    const h = harness(), button = el('button', {hidden: '', 'data-dialog-initial-open': '', 'data-dialog-open': target}); h.body.append(button);
    const event = h.click(button); assert.equal(event.defaultPrevented, false); assert.equal(event.stopped, false); assert.equal(h.requests.length, 0);
  }
  const h = harness(), unsupported = el('button', {'data-dialog-open': 'event-drawer'}); h.body.append(unsupported);
  assert.equal(h.click(unsupported).defaultPrevented, false);
  const supported = el('button', {'data-dialog-open': 'model-drawer'}); h.body.append(supported);
  assert.equal(h.click(supported, {ctrlKey: true}).defaultPrevented, false); assert.equal(h.requests.length, 0);
});

test('readonly fragments disable editing controls but preserve cancellation and namespaced form ownership', async () => {
  for (const readonly of [true, false]) {
    const h = harness(), parent = h.native(), pending = h.api.open(h.link(parent));
    const tree = fragment(), panel = tree.querySelector('.drawer-panel');
    tree.querySelector('button').type = 'submit';
    panel.append(el('select'), el('textarea'), el('button', {'data-dialog-open': 'category-drawer'}), el('button', {type: 'button', 'data-dialog-close': ''}));
    h.respond(0, {tree, readonly}); await pending;
    const remote = h.remote();
    for (const node of remote.querySelectorAll('input,select,textarea,button[type="submit"],[data-dialog-open]')) assert.equal(node.disabled, readonly);
    assert.equal(remote.querySelector('[data-dialog-close]').disabled, false);
    assert.equal(remote.querySelector('button[type="submit"]').getAttribute('form'), remote.querySelector('form').id);
    assert.equal(remote.querySelector('input').value, 'server value');
  }
});

test('query refresh preserves a chosen resource and select node even when absent from new results', async () => {
  const h = harness(), parent = h.native(), pending = h.api.open(h.link(parent));
  const tree = fragment(), original = el('select', {'data-drawer-query-select': 'resource', name: 'resource_id'}), selected = el('option');
  selected.value = 'selected-resource'; selected.textContent = 'Selected resource'; original.append(selected); original.value = selected.value;
  tree.querySelector('[data-drawer-query-results]').append(original); h.respond(0, {tree}); await pending;
  const child = h.remote(), form = child.querySelector('form'); form.method = 'get'; submit(h, form);
  const next = fragment(), replacement = el('select', {'data-drawer-query-select': 'resource', name: 'resource_id'}), candidate = el('option');
  candidate.value = 'other-resource'; replacement.append(candidate); next.querySelector('[data-drawer-query-results]').append(replacement);
  next.querySelector('[data-drawer-query-results]').append(el('script'));
  h.respond(1, {tree: next}); await settle();
  assert.equal(child.querySelector('select'), original); assert.equal(original.value, 'selected-resource');
  assert.deepEqual(original.options.map(option => option.value), ['other-resource', 'selected-resource']);
  assert.equal(child.querySelector('form'), form);
  assert.equal(child.querySelector('script'), null, 'query result replacements also remove scripts');
});

test('explicit initial focus wins over earlier buttons and inputs', async () => {
  const h = harness(), parent = h.native(), pending = h.api.open(h.link(parent));
  const tree = fragment(), preferred = el('select', {'data-dialog-initial-focus': ''});
  tree.querySelector('form').append(preferred); h.respond(0, {tree}); await pending;
  const child = h.remote(); assert.equal(preferred.focused, true);
  assert.notEqual(child.querySelector('button').focused, true); assert.notEqual(child.querySelector('input').focused, true);
});
