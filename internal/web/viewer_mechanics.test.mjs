import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import * as math from './static/vendor/three-0.180.0/three.module.min.js';

const source = readFileSync(new URL('./static/asset-model-viewer.js', import.meta.url), 'utf8').replace(/^import .*;\r?\n/gm, '').replace('export function initializeViewers','function initializeViewers');

test('viewer stage inherits the theme accent surface without an opaque WebGL background', () => {
  const css = readFileSync(new URL('./static/app.css', import.meta.url), 'utf8');
  assert.match(css, /--product-stage:\s*var\(--accent-soft\)/);
  assert.match(css, /background:var\(--product-stage\)/);
  assert.match(source, /alpha: true/);
  assert.doesNotMatch(source, /scene\.background\s*=|setClearColor\(/);
  assert.match(css, /--accent-soft:var\(--accent-light-soft\)/);
  assert.equal((css.match(/--accent-soft:var\(--accent-dark-soft\)/g) || []).length, 2);
});

function events() {
  const listeners = new Map();
  return {
    addEventListener(name, fn, options = {}) {
      if (!listeners.has(name)) listeners.set(name, new Map());
      listeners.get(name).set(fn, options);
    },
    removeEventListener(name, fn) { listeners.get(name)?.delete(fn); },
    emit(name, detail) {
      for (const [fn, options] of [...(listeners.get(name) || [])]) {
        if (options.once) listeners.get(name).delete(fn);
        fn({ type: name, detail });
      }
    },
    count(name) { return listeners.get(name)?.size || 0; },
  };
}

function viewer({ reduced = false, webgl = true, width = 500, height = 300, dimensions = [2, 2, 2], dynamic = false, inDrawer = false } = {}) {
  const listeners = new Map();
  const frames = new Map();
  const classes = new Set();
  const state = { loads: 0, draws: 0, disposed: 0, controlsDisposed: 0, resizeDisconnected: 0, intersectionDisconnected: 0 };
  const drawer = inDrawer ? events() : null;
  const canvas = { hidden: false, addEventListener: (name, callback) => listeners.set(name, callback) };
  const status = { hidden: false, textContent: '' };
  const reset = { addEventListener: (_name, callback) => { state.reset = callback; } };
  const rotate = { addEventListener: (_name, callback) => { state.toggle = callback; }, setAttribute: (_name, value) => { state.pressed = value; } };
  const motionEvents = events();
  const motion = { ...motionEvents, matches: reduced, addEventListener: (name, fn) => { state.motion = fn; motionEvents.addEventListener(name, fn); } };
  const root = {
    closest: () => drawer,
    dataset: { modelUrl: '/selected.glb', modelError: 'Localized fallback', modelLoaded: 'Localized ready' },
    querySelector: (selector) => ({ '[data-model-canvas]': canvas, '[data-model-status]': status, '[data-model-reset]': reset, '[data-model-rotate]': rotate })[selector],
    getBoundingClientRect: () => ({ width, height }),
    classList: { add: (name) => classes.add(name), remove: (name) => classes.delete(name) },
  };
  class Renderer {
    constructor() { if (!webgl) throw new Error('WebGL unavailable'); }
    setPixelRatio() {} setSize() {}
    render(_scene, camera) {
      state.draws++; state.position = camera.position.clone();
      state.camera = camera.clone();
      state.camera.lookAt(0, 0, 0);
      state.camera.updateMatrixWorld();
    }
    dispose() { state.disposed++; }
  }
  class Controls {
    constructor() { this.target = new math.Vector3(); state.controls = this; }
    update(delta) { state.delta = delta; return false; }
    addEventListener(name, fn) { this[name] = fn; }
    dispose() { state.controlsDisposed++; }
  }
  class Loader {
    load(url, success, _progress, failure) { state.loads++; state.url = url; state.success = success; state.failure = failure; }
  }
  let nextFrame = 0;
  let time = 0;
  const docEvents = events();
  const doc = { ...docEvents, hidden: false, querySelectorAll: () => dynamic ? [] : [root], addEventListener: (name, fn) => { state.visibility = fn; docEvents.addEventListener(name, fn); } };
  const win = { ...events(), devicePixelRatio: 1, matchMedia: () => motion, setTimeout() {} };
  const context = vm.createContext({
    THREE: { ...math, WebGLRenderer: Renderer }, OrbitControls: Controls, GLTFLoader: Loader,
    document: doc,
    window: win,
    ResizeObserver: class { constructor(fn) { state.resize = fn; } observe() {} disconnect() { state.resizeDisconnected++; } },
    IntersectionObserver: class { constructor(fn) { state.intersection = fn; } observe() {} disconnect() { state.intersectionDisconnected++; } },
    requestAnimationFrame: (fn) => { frames.set(++nextFrame, fn); return nextFrame; },
    cancelAnimationFrame: (id) => frames.delete(id),
  });
  vm.runInContext(source, context);
  const flush = () => {
    let iterations = 0;
    while (frames.size) {
      if (++iterations > 10) {
        if (state.controls.autoRotate) break;
        throw new Error('Viewer never becomes idle');
      }
      time += 16;
      const batch = [...frames.values()]; frames.clear(); batch.forEach((fn) => fn(time));
    }
  };
  return {
    state, status, canvas, classes, motion, flush, doc, rotate, drawer, win, frames,
    initialize() { context.initializeViewers({ querySelectorAll: () => [root] }); },
    resize(w, h) { width = w; height = h; state.resize(); flush(); },
    ready(scene = new math.Mesh(new math.BoxGeometry(...dimensions))) { state.success({ scene }); flush(); },
    key(key) { let prevented = false; listeners.get('keydown')({ key, preventDefault() { prevented = true; } }); flush(); return prevented; },
  };
}

test('keyboard rotates, zooms within limits, and restores the initial view', () => {
  const v = viewer(); v.ready();
  const home = v.state.position.clone();
  assert.equal(v.key('ArrowLeft'), true);
  assert.ok(v.state.position.distanceTo(home) > 0.01);
  const distance = v.state.position.length();
  v.key('+'); assert.ok(v.state.position.length() < distance);
  for (let i = 0; i < 100; i++) v.key('+');
  assert.ok(v.state.position.length() >= v.state.controls.minDistance - 1e-9);
  v.key('Home'); assert.ok(v.state.position.distanceTo(home) < 1e-9);
  assert.equal(v.key('Tab'), false);
  assert.equal(v.status.textContent, 'Localized ready');
});

test('reduced motion disables damping, updates live, and rendering becomes idle', () => {
  const v = viewer({ reduced: true }); v.ready();
  assert.equal(v.state.controls.enableDamping, false);
  const before = v.state.draws; v.flush(); assert.equal(v.state.draws, before);
  v.motion.matches = false; v.state.motion(); v.flush();
  assert.equal(v.state.controls.enableDamping, true);
  assert.equal(v.state.controls.autoRotate, true);
  v.motion.matches = true; v.state.motion(); v.flush();
  assert.equal(v.state.controls.autoRotate, false);
  assert.equal(v.rotate.disabled, true);
  const stopped = v.state.draws; v.flush(); assert.equal(v.state.draws, stopped);
});

test('slow autoplay pauses for user controls, offscreen and hidden pages', () => {
  const v = viewer(); v.ready();
  assert.equal(v.state.controls.autoRotate, true);
  assert.equal(v.state.controls.autoRotateSpeed, 0.5);
  assert.equal(v.state.delta, 0.016);
  assert.equal(v.state.pressed, 'true');
  v.state.toggle(); v.flush();
  assert.equal(v.state.controls.autoRotate, false);
  assert.equal(v.state.pressed, 'false');
  const stopped = v.state.draws; v.flush(); assert.equal(v.state.draws, stopped);
  v.state.toggle(); v.flush();
  v.state.controls.start(); v.flush(); assert.equal(v.state.controls.autoRotate, false);
  v.state.controls.end(); v.flush(); assert.equal(v.state.controls.autoRotate, true);
  v.state.intersection([{ isIntersecting: false }]); v.flush();
  assert.equal(v.state.controls.autoRotate, false);
  const offscreen = v.state.draws; v.flush(); assert.equal(v.state.draws, offscreen);
  v.state.intersection([{ isIntersecting: true }]); v.flush(); assert.equal(v.state.controls.autoRotate, true);
  v.doc.hidden = true; v.state.visibility(); v.flush(); assert.equal(v.state.controls.autoRotate, false);
  v.doc.hidden = false; v.state.visibility(); v.flush(); assert.equal(v.state.controls.autoRotate, true);
});

test('dynamic drawer initialization loads once across repeated initialization', () => {
  const v = viewer({ dynamic: true, inDrawer: true });
  assert.equal(v.state.loads, 0, 'A detached drawer is not initialized on page load');
  v.initialize();
  assert.equal(v.state.loads, 1);
  v.ready();
  const controls = v.state.controls;
  v.initialize(); v.initialize();
  assert.equal(v.state.loads, 1, 'Reinitializing the same node must not fetch again');
  assert.equal(v.state.controls, controls, 'Controls must not be duplicated');
  assert.equal(v.drawer.count('drawer:dispose'), 1);
  assert.equal(v.doc.count('visibilitychange'), 1);
  assert.equal(v.classes.has('is-ready'), true);
});

test('covered drawer stops drawing and resumes without losing the user pause preference', () => {
  const v = viewer({ inDrawer: true }); v.ready();
  assert.equal(v.state.controls.autoRotate, true);
  v.drawer.emit('drawer:visibility', { visible: false });
  const before = v.state.draws;
  v.flush();
  assert.equal(v.state.controls.autoRotate, false);
  assert.equal(v.state.draws, before, 'A queued frame cannot draw a covered viewer');
  assert.equal(v.frames.size, 0);
  v.drawer.emit('drawer:visibility', { visible: true }); v.flush();
  assert.equal(v.state.controls.autoRotate, true);
  assert.ok(v.state.draws > before);
  v.state.toggle(); v.flush();
  assert.equal(v.state.pressed, 'false');
  v.drawer.emit('drawer:visibility', { visible: false }); v.flush();
  v.drawer.emit('drawer:visibility', { visible: true }); v.flush();
  assert.equal(v.state.controls.autoRotate, false, 'Uncovering must retain manual pause');
  assert.equal(v.state.pressed, 'false');
  const paused = v.state.draws; v.flush(); assert.equal(v.state.draws, paused);
});

test('a model loaded while covered waits for uncovering before rendering', () => {
  const v = viewer({ inDrawer: true });
  v.drawer.emit('drawer:visibility', { visible: false });
  v.ready();
  assert.equal(v.state.draws, 0);
  assert.equal(v.state.controls.autoRotate, false);
  v.drawer.emit('drawer:visibility', { visible: true }); v.flush();
  assert.ok(v.state.draws > 0);
  assert.equal(v.state.loads, 1);
});

function trackedModel() {
  const released = { geometry: 0, materials: 0, textures: 0 };
  const geometry = new math.BoxGeometry(2, 2, 2);
  geometry.addEventListener('dispose', () => released.geometry++);
  const texture = new math.Texture();
  texture.addEventListener('dispose', () => released.textures++);
  const materials = [new math.MeshBasicMaterial({ map: texture }), new math.MeshBasicMaterial({ map: texture })];
  for (const material of materials) material.addEventListener('dispose', () => released.materials++);
  return { scene: new math.Mesh(geometry, materials), released };
}

test('drawer disposal releases GPU resources, observers and global listeners and cancels rendering', () => {
  const v = viewer({ inDrawer: true });
  const model = trackedModel(); v.ready(model.scene);
  assert.ok(v.frames.size > 0);
  v.drawer.emit('drawer:dispose');
  const draws = v.state.draws; v.flush();
  assert.equal(v.state.draws, draws);
  assert.equal(v.frames.size, 0);
  assert.equal(v.state.disposed, 1);
  assert.equal(v.state.controlsDisposed, 1);
  assert.equal(v.state.resizeDisconnected, 1);
  assert.equal(v.state.intersectionDisconnected, 1);
  assert.equal(v.doc.count('visibilitychange'), 0);
  assert.equal(v.motion.count('change'), 0);
  assert.equal(v.win.count('pagehide'), 0);
  assert.deepEqual(model.released, { geometry: 1, materials: 2, textures: 1 });
  v.drawer.emit('drawer:dispose'); v.win.emit('pagehide');
  assert.equal(v.state.disposed, 1, 'Removed/once listeners must not dispose twice');
  v.drawer.emit('drawer:visibility', { visible: true }); v.flush();
  assert.equal(v.state.draws, draws, 'A stale visibility event cannot restart drawing');
});

test('late GLB arrival after drawer disposal is released without rendering or revealing the model', () => {
  const v = viewer({ inDrawer: true });
  v.drawer.emit('drawer:dispose');
  const model = trackedModel(); v.ready(model.scene);
  assert.deepEqual(model.released, { geometry: 1, materials: 2, textures: 1 });
  assert.equal(v.state.draws, 0);
  assert.equal(v.frames.size, 0);
  assert.equal(v.classes.has('is-ready'), false);
  assert.equal(v.status.textContent, '');
  assert.equal(v.state.disposed, 1);
});

test('both viewer surfaces expose a keyboard-accessible autoplay toggle', () => {
  for (const template of ['asset', 'resource']) {
    const html = readFileSync(new URL(`./templates/${template}.html`, import.meta.url), 'utf8');
    assert.match(html, /<button[^>]*data-model-rotate[^>]*aria-pressed="true"/);
    assert.match(html, /resource.auto_rotate/);
    const toggle = html.match(/<button[^>]*data-model-rotate[\s\S]*?<\/button>/)?.[0];
    assert.match(toggle, /aria-label=/);
    assert.match(toggle, /class="model-pause"/);
    assert.match(toggle, /class="model-play"/);
    assert.equal(toggle.replace(/<[^>]*>/g, '').trim(), '');
    const reset = html.match(/<button[^>]*data-model-reset[\s\S]*?<\/button>/)?.[0];
    assert.ok(reset, 'Reset button is present');
    assert.match(reset, /class="icon-button"/);
    assert.match(reset, /aria-label="{{t \$s "asset.model_3d_reset"}}"/);
    assert.match(reset, /title="{{t \$s "asset.model_3d_reset"}}"/);
    assert.match(reset, /<svg[^>]*aria-hidden="true"/);
    assert.equal(reset.replace(/<[^>]*>/g, '').trim(), '', 'Reset is icon-only');
  }
});

test('resize refits reset framing while preserving the user zoom ratio', () => {
  const v = viewer({ reduced: true, width: 500, height: 500, dimensions: [1, 7, 1] }); v.ready();
  v.key('+');
  v.resize(240, 300);
  const zoomedDistance = v.state.position.length();
  v.state.reset(); v.flush();
  assert.ok(Math.abs(zoomedDistance / v.state.position.length() - 0.9) < 1e-9);
});

test('compact controls reveal on hover or keyboard focus and remain available on touch', () => {
  const css = readFileSync(new URL('./static/app.css', import.meta.url), 'utf8');
  assert.match(css, /@media \(hover:hover\) and \(pointer:fine\)/);
  assert.match(css, /\.asset-product-visual:focus-within \.asset-model-controls/);
  assert.match(css, /flex-wrap:nowrap/);
});

test('selected-resource failure leaves the fallback and never loads another resource', () => {
  const v = viewer(); v.state.failure(); v.flush();
  assert.equal(v.state.loads, 1);
  assert.equal(v.state.url, '/selected.glb');
  assert.equal(v.canvas.hidden, true);
  assert.equal(v.classes.has('is-ready'), false);
  assert.equal(v.status.textContent, 'Localized fallback');
  assert.equal(v.status.hidden, false);
});

test('WebGL unavailable preserves fallback without fetching the resource', () => {
  const v = viewer({ webgl: false });
  assert.equal(v.state.loads, 0);
  assert.equal(v.canvas.hidden, true);
  assert.equal(v.status.textContent, 'Localized fallback');
});

test('initial framing fits every bounding-box corner on narrow and wide stages with control clearance', () => {
  for (const [width, height] of [[240, 500], [1200, 260]]) {
    for (const dimensions of [[2, 2, 2], [8, 1, 3], [1, 7, 1]]) {
      const v = viewer({ width, height, dimensions }); v.ready();
      let extent = 0;
      for (const x of [-1, 1]) for (const y of [-1, 1]) for (const z of [-1, 1]) {
        const corner = new math.Vector3(x * dimensions[0] / 2, y * dimensions[1] / 2, z * dimensions[2] / 2).project(v.state.camera);
        extent = Math.max(extent, Math.abs(corner.x), Math.abs(corner.y));
        assert.ok(Math.abs(corner.x) <= 0.750001 && Math.abs(corner.y) <= 0.750001,
          `Clipped or crowded corner at ${width}×${height}: ${corner.toArray()}`);
        assert.ok(corner.z > -1 && corner.z < 1, 'Corner outside camera clipping planes');
      }
      assert.ok(Math.abs(extent - 0.75) < 0.00001, 'Initial view must fill 75% on the limiting axis');
    }
  }
});
