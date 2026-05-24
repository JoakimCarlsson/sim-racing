import {
  BoxGeometry,
  DirectionalLight,
  HemisphereLight,
  Mesh,
  MeshStandardMaterial,
  PerspectiveCamera,
  PlaneGeometry,
  Scene,
  WebGLRenderer,
} from "three";
import { Socket } from "./net/socket";

// --- WebSocket echo loop ---
// Initialised first so a WebGL failure cannot prevent WS from connecting.
// send("hello") before connect() queues the message; Socket.onopen flushes it.
const sock = new Socket();
sock.onMessage((data) => {
  console.log("received:", data);
});
sock.send("hello");
sock.connect();

// --- Three.js renderer (wrapped so a headless/GPU failure is non-fatal) ---
try {
  const renderer = new WebGLRenderer({ antialias: true });
  renderer.setPixelRatio(window.devicePixelRatio);
  renderer.setSize(window.innerWidth, window.innerHeight);
  document.body.appendChild(renderer.domElement);

  const camera = new PerspectiveCamera(
    60,
    window.innerWidth / window.innerHeight,
    0.1,
    100,
  );
  camera.position.set(0, 3, 6);
  camera.lookAt(0, 0.5, 0);

  const scene = new Scene();

  const hemiLight = new HemisphereLight(0xffffff, 0x444444, 1.5);
  scene.add(hemiLight);

  const dirLight = new DirectionalLight(0xffffff, 1.0);
  dirLight.position.set(5, 10, 7.5);
  scene.add(dirLight);

  const groundGeometry = new PlaneGeometry(20, 20);
  const groundMaterial = new MeshStandardMaterial({ color: 0x888888 });
  const ground = new Mesh(groundGeometry, groundMaterial);
  ground.rotation.x = -Math.PI / 2;
  scene.add(ground);

  const cubeGeometry = new BoxGeometry(1, 1, 1);
  const cubeMaterial = new MeshStandardMaterial({ color: 0x44aa88 });
  const cube = new Mesh(cubeGeometry, cubeMaterial);
  cube.position.y = 0.5;
  scene.add(cube);

  function onResize(): void {
    camera.aspect = window.innerWidth / window.innerHeight;
    camera.updateProjectionMatrix();
    renderer.setSize(window.innerWidth, window.innerHeight);
  }

  window.addEventListener("resize", onResize);

  function animate(): void {
    requestAnimationFrame(animate);
    cube.rotation.x += 0.01;
    cube.rotation.y += 0.013;
    renderer.render(scene, camera);
  }

  animate();
} catch (err) {
  console.warn("WebGL unavailable — 3-D rendering disabled:", err);
}
