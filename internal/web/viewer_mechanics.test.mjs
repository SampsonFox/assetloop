import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import * as math from './static/vendor/three-0.180.0/three.module.min.js';

const source = readFileSync(new URL('./static/asset-model-viewer.js', import.meta.url), 'utf8').replace(/^import .*;\r?\n/gm, '');

test('viewer stage inherits the theme accent surface without an opaque WebGL background', () => {
  const css = readFileSync(new URL('./static/app.css', import.meta.url), 'utf8');
  assert.match(css, /--product-stage:\s*var\(--accent-soft\)/);
  assert.match(css, /background:var\(--product-stage\)/);
  assert.match(source, /alpha: true/);
  assert.doesNotMatch(source, /scene\.background\s*=|setClearColor\(/);
  assert.match(css, /--accent-soft:var\(--accent-light-soft\)/);
  assert.equal((css.match(/--accent-soft:var\(--accent-dark-soft\)/g) || []).length, 2);
});

function viewer({ reduced = false, webgl = true, width = 500, height = 300, dimensions = [2, 2, 2] } = {}) {
  const listeners = new Map();
  const frames = new Map();
  const classes = new Set();
  const state = { loads: 0, draws: 0, disposed: 0 };
  const canvas = { hidden: false, addEventListener: (name, callback) => listeners.set(name, callback) };
  const status = { hidden: false, textContent: '' };
  const reset = { addEventListener: (_name, callback) => { state.reset = callback; } };
  const rotate = { addEventListener: (_name, callback) => { state.toggle = callback; }, setAttribute: (_name, value) => { state.pressed = value; } };
  const motion = { matches: reduced, addEventListener: (_name, fn) => { state.motion = fn; }, removeEventListener() {} };
  const root = {
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
    dispose() {}
  }
  class Loader {
    load(url, success, _progress, failure) { state.loads++; state.url = url; state.success = success; state.failure = failure; }
  }
  let nextFrame = 0;
  let time = 0;
  const doc = { hidden: false, querySelectorAll: () => [root], addEventListener: (_name, fn) => { state.visibility = fn; }, removeEventListener() {} };
  vm.runInNewContext(source, {
    THREE: { ...math, WebGLRenderer: Renderer }, OrbitControls: Controls, GLTFLoader: Loader,
    document: doc,
    window: { devicePixelRatio: 1, matchMedia: () => motion, setTimeout() {}, addEventListener() {} },
    ResizeObserver: class { observe() {} disconnect() {} },
    IntersectionObserver: class { constructor(fn) { state.intersection = fn; } observe() {} disconnect() {} },
    requestAnimationFrame: (fn) => { frames.set(++nextFrame, fn); return nextFrame; },
    cancelAnimationFrame: (id) => frames.delete(id),
  });
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
    state, status, canvas, classes, motion, flush, doc, rotate,
    ready() { state.success({ scene: new math.Mesh(new math.BoxGeometry(...dimensions)) }); flush(); },
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

test('both viewer surfaces expose a keyboard-accessible autoplay toggle', () => {
  for (const template of ['asset', 'resource']) {
    const html = readFileSync(new URL(`./templates/${template}.html`, import.meta.url), 'utf8');
    assert.match(html, /<button[^>]*data-model-rotate[^>]*aria-pressed="true"/);
    assert.match(html, /resource.auto_rotate/);
  }
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
      for (const x of [-1, 1]) for (const y of [-1, 1]) for (const z of [-1, 1]) {
        const corner = new math.Vector3(x * dimensions[0] / 2, y * dimensions[1] / 2, z * dimensions[2] / 2).project(v.state.camera);
        assert.ok(Math.abs(corner.x) <= 0.7 && Math.abs(corner.y) <= 0.7,
          `Clipped or crowded corner at ${width}×${height}: ${corner.toArray()}`);
        assert.ok(corner.z > -1 && corner.z < 1, 'Corner outside camera clipping planes');
      }
    }
  }
});
