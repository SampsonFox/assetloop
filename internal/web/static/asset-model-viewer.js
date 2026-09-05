import * as THREE from './vendor/three-0.180.0/three.module.min.js';
import { GLTFLoader } from './vendor/three-0.180.0/GLTFLoader.js';
import { OrbitControls } from './vendor/three-0.180.0/OrbitControls.js';

for (const root of document.querySelectorAll('[data-model-viewer]')) {
  const canvas = root.querySelector('[data-model-canvas]');
  const status = root.querySelector('[data-model-status]');
  const reset = root.querySelector('[data-model-reset]');
  const rotate = root.querySelector('[data-model-rotate]');
  let renderer;
  let controls;
  let observer;
  let frame = 0;
  let failed = false;
  let visibilityObserver;
  let visible = true;
  let interacting = false;
  let paused = false;
  let previousTime = null;

  try {
    renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true, powerPreference: 'high-performance' });
  } catch {
    fail(root.dataset.modelError);
    continue;
  }

  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
  renderer.outputColorSpace = THREE.SRGBColorSpace;
  renderer.toneMapping = THREE.ACESFilmicToneMapping;
  renderer.toneMappingExposure = 1.1;

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(36, 1, 0.01, 1000);
  controls = new OrbitControls(camera, canvas);
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
  controls.enableDamping = !reducedMotion.matches;
  controls.autoRotateSpeed = 0.5; // One revolution in two minutes, independent of refresh rate.
  controls.dampingFactor = 0.08;
  controls.enablePan = false;
  controls.minDistance = 0.1;
  controls.maxDistance = 100;
  scene.add(new THREE.HemisphereLight(0xffffff, 0x65736c, 2.2));
  const key = new THREE.DirectionalLight(0xffffff, 3.2);
  key.position.set(3, 5, 4);
  scene.add(key);
  const fill = new THREE.DirectionalLight(0xb8d8ff, 1.4);
  fill.position.set(-4, 1, -3);
  scene.add(fill);

  let home = null;
  let modelSize = null;
  const fitView = () => {
    if (!modelSize) return;
    const direction = new THREE.Vector3(0.72, 0.2, 1).normalize();
    const right = new THREE.Vector3().crossVectors(new THREE.Vector3(0, 1, 0), direction).normalize();
    const up = new THREE.Vector3().crossVectors(direction, right);
    const tanV = Math.tan(THREE.MathUtils.degToRad(camera.fov * 0.5));
    let distance = 0;
    // Fit the projected box to 75% of the canvas, not an oversized enclosing sphere.
    for (const x of [-1, 1]) for (const y of [-1, 1]) for (const z of [-1, 1]) {
      const corner = new THREE.Vector3(x * modelSize.x / 2, y * modelSize.y / 2, z * modelSize.z / 2);
      distance = Math.max(distance, corner.dot(direction) + Math.max(Math.abs(corner.dot(right)) / (tanV * camera.aspect * 0.75), Math.abs(corner.dot(up)) / (tanV * 0.75)));
    }
    const radius = modelSize.length() * 0.5 || 1;
    distance = Math.max(distance, radius);
    const zoom = home ? camera.position.distanceTo(controls.target) / home.position.length() : 1;
    const currentDirection = home ? camera.position.clone().sub(controls.target).normalize() : direction;
    home = { position: direction.clone().multiplyScalar(distance), target: new THREE.Vector3() };
    camera.near = Math.max(radius / 100, 0.001);
    camera.far = Math.max(distance * 20, 100);
    controls.minDistance = radius * 0.7;
    controls.maxDistance = distance * 4;
    camera.position.copy(controls.target).add(currentDirection.clone().multiplyScalar(Math.max(controls.minDistance, Math.min(controls.maxDistance, distance * zoom))));
    camera.updateProjectionMatrix();
  };
  const render = (time) => {
    frame = 0;
    if (failed || !visible || document.hidden) { previousTime = null; return; }
    const delta = previousTime === null ? 0 : Math.min((time - previousTime) / 1000, 0.05);
    previousTime = time;
    const moving = controls.update(delta);
    renderer.render(scene, camera);
    if (controls.autoRotate || (moving && controls.enableDamping)) requestRender();
    else previousTime = null;
  };
  const requestRender = () => { if (!frame && !failed && visible && !document.hidden) frame = requestAnimationFrame(render); };
  controls.addEventListener('change', requestRender);
  const motionChanged = () => {
    controls.enableDamping = !reducedMotion.matches;
    controls.autoRotate = !!home && !paused && !reducedMotion.matches && !interacting && visible && !document.hidden;
    if (rotate) {
      rotate.disabled = reducedMotion.matches;
      rotate.setAttribute('aria-pressed', String(!paused && !reducedMotion.matches));
    }
    previousTime = null;
    requestRender();
  };
  reducedMotion.addEventListener('change', motionChanged);
  rotate?.addEventListener('click', () => { paused = !paused; motionChanged(); });
  controls.addEventListener('start', () => { interacting = true; motionChanged(); });
  controls.addEventListener('end', () => { interacting = false; motionChanged(); });
  document.addEventListener('visibilitychange', motionChanged);
  visibilityObserver = new IntersectionObserver(([entry]) => { visible = entry.isIntersecting; motionChanged(); });
  visibilityObserver.observe(root);
  motionChanged();
  const resize = () => {
    const { width, height } = root.getBoundingClientRect();
    if (!width || !height) return;
    const canvasHeight = Math.max(height - 56, 1);
    renderer.setSize(width, canvasHeight, false);
    camera.aspect = width / canvasHeight;
    fitView();
    camera.updateProjectionMatrix();
    requestRender();
  };
  observer = new ResizeObserver(resize);
  observer.observe(root);
  resize();

  new GLTFLoader().load(root.dataset.modelUrl, (gltf) => {
    const object = gltf.scene;
    const box = new THREE.Box3().setFromObject(object);
    if (box.isEmpty()) { fail(root.dataset.modelError); return; }
    const center = box.getCenter(new THREE.Vector3());
    const size = box.getSize(new THREE.Vector3());
    object.position.sub(center);
    scene.add(object);
    controls.target.set(0, 0, 0);
    modelSize = size;
    fitView();
    controls.update();
    motionChanged();
    root.classList.add('is-ready');
    status.textContent = root.dataset.modelLoaded;
    window.setTimeout(() => { if (!failed) status.hidden = true; }, 1200);
    requestRender();
  }, undefined, () => fail(root.dataset.modelError));

  const resetView = () => {
    if (!home) return;
    camera.position.copy(home.position);
    controls.target.copy(home.target);
    controls.update(0);
    requestRender();
  };
  reset?.addEventListener('click', resetView);
  canvas.addEventListener('keydown', (event) => {
    if (!home || failed) return;
    if (event.key === 'Home' || event.key.toLowerCase() === 'r') {
      event.preventDefault(); resetView(); return;
    }
    if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', '+', '=', '-', '_'].includes(event.key)) return;
    event.preventDefault();
    const offset = camera.position.clone().sub(controls.target);
    const spherical = new THREE.Spherical().setFromVector3(offset);
    if (event.key === 'ArrowLeft') spherical.theta -= 0.12;
    if (event.key === 'ArrowRight') spherical.theta += 0.12;
    if (event.key === 'ArrowUp') spherical.phi -= 0.12;
    if (event.key === 'ArrowDown') spherical.phi += 0.12;
    if (event.key === '+' || event.key === '=') spherical.radius *= 0.9;
    if (event.key === '-' || event.key === '_') spherical.radius *= 1.1;
    spherical.radius = Math.max(controls.minDistance, Math.min(controls.maxDistance, spherical.radius));
    spherical.makeSafe();
    camera.position.copy(controls.target).add(offset.setFromSpherical(spherical));
    controls.update(0);
    requestRender();
  });

  window.addEventListener('pagehide', () => {
    failed = true;
    cancelAnimationFrame(frame);
    observer.disconnect();
    visibilityObserver.disconnect();
    document.removeEventListener('visibilitychange', motionChanged);
    reducedMotion.removeEventListener('change', motionChanged);
    controls.dispose();
    renderer.dispose();
  }, { once: true });

  function fail(message) {
    failed = true;
    cancelAnimationFrame(frame);
    root.classList.remove('is-ready');
    status.textContent = message;
    status.hidden = false;
    canvas.hidden = true;
    observer?.disconnect();
    visibilityObserver?.disconnect();
    controls?.dispose();
    if (renderer) renderer.dispose();
  }
}
