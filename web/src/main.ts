/**
 * web/src/main.ts — Application entry point.
 *
 * Boot order:
 *   1. Load WASM physics.
 *   2. Construct Predictor.
 *   3. Set up WebSocket + InputSender with onTick wired to predictor.tick().
 *   4. Wire binary messages: 0x03 → decodeServerHello (store ownPlayerID);
 *      0x02 → decodeServerSnapshot (predictor.applySnapshot).
 *   5. Bind Three.js cube transform to predictor.predictedState each frame.
 *
 * Non-goals: reconciliation, real car mesh, gamepad, ServerHello.Constants
 * wiring (defaultConstants used — we warn if server sends non-default values).
 */

import {
  BoxGeometry,
  DirectionalLight,
  HemisphereLight,
  Mesh,
  MeshStandardMaterial,
  PerspectiveCamera,
  PlaneGeometry,
  Quaternion,
  Scene,
  Vector3,
  WebGLRenderer,
} from 'three';
import { Socket } from './net/socket';
import { KeyboardSource } from './input/keyboard';
import { InputSampler } from './input/sampler';
import { InputSender } from './net/inputSender';
import { loadPhysics } from './physics/wasm';
import { Predictor } from './sim/predictor';
import {
  MsgType,
  decodeServerHello,
  decodeServerSnapshot,
} from './protocol/messages';

async function main(): Promise<void> {
  // -------------------------------------------------------------------------
  // 1. Load WASM physics
  // -------------------------------------------------------------------------
  const physics = await loadPhysics('/physics.wasm', '/wasm_exec.js');

  // -------------------------------------------------------------------------
  // 2. Construct Predictor
  // -------------------------------------------------------------------------
  const predictor = new Predictor(physics);

  // ownPlayerID is set when ServerHello (0x03) arrives.
  let ownPlayerID: number | null = null;

  // -------------------------------------------------------------------------
  // 3. WebSocket + input pipeline
  // -------------------------------------------------------------------------
  const wsUrl = import.meta.env.VITE_SERVER_WS_URL;
  const sock = new Socket(wsUrl);

  const keyboard = new KeyboardSource();
  const sampler = new InputSampler(keyboard);

  const sender = new InputSender(sampler, sock, {
    now: () => performance.now(),
    onTick: (seq, input) => {
      // Advance prediction AFTER the frame is confirmed sent.
      predictor.tick(seq, input);
    },
  });

  sock.onOpen(() => {
    sampler.reset();
    sender.start();
  });

  sock.onClose(() => {
    sender.stop();
  });

  // -------------------------------------------------------------------------
  // 4. Binary message dispatch
  // -------------------------------------------------------------------------
  sock.onBinaryMessage((data: ArrayBuffer) => {
    const buf = new Uint8Array(data);
    if (buf.byteLength < 1) return;

    const msgType = buf[0];

    if (msgType === MsgType.ServerHello) {
      // 0x03 — store own player ID.
      try {
        const hello = decodeServerHello(buf);
        ownPlayerID = hello.playerID;
        // Non-goal: we use defaultConstants for prediction.
        // Warn if the server sends something unexpected (e.g., non-default mass).
        const defaultC = physics.defaultConstants();
        if (Math.abs(hello.constants.mass - defaultC.mass) > 0.5) {
          console.warn(
            '[predictor] ServerHello.Constants.mass differs from defaultConstants; ' +
              'prediction may diverge. Reconciliation not implemented.',
          );
        }
        console.log(`[predictor] Connected as playerID=${ownPlayerID}`);
      } catch (err) {
        console.error('[predictor] Failed to decode ServerHello:', err);
      }
      return;
    }

    if (msgType === MsgType.ServerSnapshot) {
      // 0x02 — apply to predictor if we know our playerID.
      if (ownPlayerID === null) return;
      try {
        const snap = decodeServerSnapshot(buf);
        predictor.applySnapshot(snap, ownPlayerID);
      } catch (err) {
        console.error('[predictor] Failed to decode ServerSnapshot:', err);
      }
      return;
    }

    // Unknown message type — log and ignore.
    console.log('[ws] Unknown binary message type:', msgType.toString(16));
  });

  sock.connect();

  // -------------------------------------------------------------------------
  // 5. Three.js renderer
  // -------------------------------------------------------------------------
  try {
    const renderer = new WebGLRenderer({ antialias: true });
    renderer.setPixelRatio(window.devicePixelRatio);
    renderer.setSize(window.innerWidth, window.innerHeight);
    document.body.appendChild(renderer.domElement);

    const camera = new PerspectiveCamera(60, window.innerWidth / window.innerHeight, 0.1, 100);
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

    const _pos = new Vector3();
    const _quat = new Quaternion();

    function onResize(): void {
      camera.aspect = window.innerWidth / window.innerHeight;
      camera.updateProjectionMatrix();
      renderer.setSize(window.innerWidth, window.innerHeight);
    }

    window.addEventListener('resize', onResize);

    function animate(): void {
      requestAnimationFrame(animate);

      // Bind cube transform to render state (predicted + visual smoothing offset).
      const ps = predictor.renderState;
      _pos.set(ps.position[0], ps.position[1], ps.position[2]);

      // Clamp y so the cube never sinks below the ground plane.
      if (_pos.y < 0.5) _pos.y = 0.5;

      cube.position.copy(_pos);

      // orientation is [qx, qy, qz, qw].
      _quat.set(
        ps.orientation[0],
        ps.orientation[1],
        ps.orientation[2],
        ps.orientation[3],
      );
      cube.quaternion.copy(_quat);

      renderer.render(scene, camera);
    }

    animate();
  } catch (err) {
    console.warn('WebGL unavailable — 3-D rendering disabled:', err);
  }
}

main().catch((err) => {
  console.error('[main] Fatal error during boot:', err);
});
