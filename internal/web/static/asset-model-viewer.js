import * as THREE from './vendor/three-0.180.0/three.module.min.js';
import { GLTFLoader } from './vendor/three-0.180.0/GLTFLoader.js';
import { OrbitControls } from './vendor/three-0.180.0/OrbitControls.js';

for (const root of document.querySelectorAll('[data-model-viewer]')) {
  const canvas = root.querySelector('[data-model-canvas]');
  const status = root.querySelector('[data-model-status]');
  const reset = root.querySelector('[data-model-reset]');
  let renderer;

  try {
    renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true, powerPreference: 'high-performance' });
  } catch {
    fail('此浏览器无法显示 3D，已显示设备图标。');
    continue;
  }

  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
  renderer.outputColorSpace = THREE.SRGBColorSpace;
  renderer.toneMapping = THREE.ACESFilmicToneMapping;
  renderer.toneMappingExposure = 1.1;

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(36, 1, 0.01, 1000);
  const controls = new OrbitControls(camera, canvas);
  controls.enableDamping = true;
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
  let frame = 0;
  const render = () => {
    controls.update();
    renderer.render(scene, camera);
    frame = requestAnimationFrame(render);
  };
  const resize = () => {
    const { width, height } = root.getBoundingClientRect();
    if (!width || !height) return;
    renderer.setSize(width, height, false);
    camera.aspect = width / height;
    camera.updateProjectionMatrix();
  };
  const observer = new ResizeObserver(resize);
  observer.observe(root);
  resize();

  new GLTFLoader().load(root.dataset.modelUrl, (gltf) => {
    const object = gltf.scene;
    const box = new THREE.Box3().setFromObject(object);
    if (box.isEmpty()) { fail('模型没有可显示内容，已显示设备图标。'); return; }
    const center = box.getCenter(new THREE.Vector3());
    const size = box.getSize(new THREE.Vector3());
    object.position.sub(center);
    scene.add(object);
    const radius = Math.max(size.x, size.y, size.z) * 0.5 || 1;
    const distance = radius / Math.tan(THREE.MathUtils.degToRad(camera.fov * 0.5)) * 1.35;
    camera.near = Math.max(radius / 100, 0.001);
    camera.far = Math.max(distance * 20, 100);
    camera.position.set(distance * 0.72, radius * 0.34, distance);
    camera.updateProjectionMatrix();
    controls.target.set(0, 0, 0);
    controls.minDistance = radius * 0.7;
    controls.maxDistance = distance * 4;
    controls.update();
    home = { position: camera.position.clone(), target: controls.target.clone() };
    root.classList.add('is-ready');
    status.textContent = '3D 模型已载入';
    window.setTimeout(() => { status.hidden = true; }, 1200);
    render();
  }, undefined, () => fail('3D 模型载入失败，已显示设备图标。'));

  reset?.addEventListener('click', () => {
    if (!home) return;
    camera.position.copy(home.position);
    controls.target.copy(home.target);
    controls.update();
  });

  window.addEventListener('pagehide', () => {
    cancelAnimationFrame(frame);
    observer.disconnect();
    controls.dispose();
    renderer.dispose();
  }, { once: true });

  function fail(message) {
    root.classList.remove('is-ready');
    status.textContent = message;
    canvas.hidden = true;
    if (renderer) renderer.dispose();
  }
}
