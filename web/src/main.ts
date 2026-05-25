/**
 * web/src/main.ts — Application entry point.
 *
 * Boot order:
 *   1. Load WASM physics.
 *   2. Construct Predictor.
 *   3. Load track.json + track.gltf; spawn car at spawnPoints[0].
 *   4. Set up WebSocket + InputSender with onTick wired to predictor.tick().
 *   5. Wire binary messages: 0x03 → decodeServerHello (store ownPlayerID);
 *      0x02 → decodeServerSnapshot (predictor.applySnapshot).
 *   6. Bind Three.js cube transform to predictor.predictedState each frame.
 *   7. Backquote key toggles the dev overlay (limits polygon + gate arrows).
 */

import * as THREE from 'three';
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js';
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
import { loadTrackJSON, loadTrackGLTF } from './track/load';
import { TrackOverlay } from './track/overlay';

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
      try {
        const hello = decodeServerHello(buf);
        ownPlayerID = hello.playerID;
        const defaultC = physics.defaultConstants();
        if (Math.abs(hello.constants.mass - defaultC.mass) > 0.5) {
          console.warn(
            '[predictor] ServerHello.Constants.mass differs from defaultConstants; ' +
              'prediction may diverge.',
          );
        }
        console.log(`[predictor] Connected as playerID=${ownPlayerID}`);
      } catch (err) {
        console.error('[predictor] Failed to decode ServerHello:', err);
      }
      return;
    }

    if (msgType === MsgType.ServerSnapshot) {
      if (ownPlayerID === null) return;
      try {
        const snap = decodeServerSnapshot(buf);
        predictor.applySnapshot(snap, ownPlayerID);
      } catch (err) {
        console.error('[predictor] Failed to decode ServerSnapshot:', err);
      }
      return;
    }

    console.log('[ws] Unknown binary message type:', msgType.toString(16));
  });

  sock.connect();

  // -------------------------------------------------------------------------
  // 5. Three.js renderer + track
  // -------------------------------------------------------------------------
  try {
    const renderer = new THREE.WebGLRenderer({ antialias: true });
    renderer.setPixelRatio(window.devicePixelRatio);
    renderer.setSize(window.innerWidth, window.innerHeight);
    document.body.appendChild(renderer.domElement);

    const camera = new THREE.PerspectiveCamera(
      60,
      window.innerWidth / window.innerHeight,
      0.1,
      200,
    );
    camera.position.set(0, 20, 30);
    camera.lookAt(0, 0, 0);

    const scene = new THREE.Scene();

    const hemiLight = new THREE.HemisphereLight(0xffffff, 0x444444, 1.5);
    scene.add(hemiLight);

    const dirLight = new THREE.DirectionalLight(0xffffff, 1.0);
    dirLight.position.set(5, 10, 7.5);
    scene.add(dirLight);

    // -----------------------------------------------------------------------
    // Load track assets
    // -----------------------------------------------------------------------
    let overlay: TrackOverlay | null = null;

    try {
      const track = await loadTrackJSON('/tracks/circuit01/track.json');
      console.log(`[track] Loaded: id=${track.id}`);

      // Load and add the track glTF mesh.
      const gltfLoader = new GLTFLoader();
      const trackGroup = await loadTrackGLTF(
        '/tracks/circuit01/track.gltf',
        gltfLoader,
      );
      scene.add(trackGroup as unknown as THREE.Object3D);
      console.log('[track] glTF mesh added to scene');

      // Create and add the dev overlay (hidden by default).
      overlay = new TrackOverlay(track, THREE as never);
      scene.add(overlay.root as unknown as THREE.Object3D);
      console.log('[track] Dev overlay constructed (Backquote to toggle)');

      // -----------------------------------------------------------------------
      // Apply spawn position from spawnPoints[0].
      // The predictor's initial _predictedState defaults to origin; override
      // it with the track's spawn so the car appears in the right place before
      // the first server snapshot arrives.
      // -----------------------------------------------------------------------
      const spawn = track.spawnPoints[0];
      // Access the private field via indexed access — TypeScript allows this
      // with bracket notation. The server will reconcile on the first snapshot.
      (predictor as unknown as {
        _predictedState: {
          position: [number, number, number];
          orientation: [number, number, number, number];
        };
      })._predictedState.position = [spawn.pos[0], spawn.pos[1], spawn.pos[2]];
      (predictor as unknown as {
        _predictedState: {
          orientation: [number, number, number, number];
        };
      })._predictedState.orientation = [
        spawn.rot[0],
        spawn.rot[1],
        spawn.rot[2],
        spawn.rot[3],
      ];
    } catch (err) {
      console.warn('[track] Failed to load track assets (continuing):', err);
    }

    // -----------------------------------------------------------------------
    // Backquote toggles the overlay
    // -----------------------------------------------------------------------
    let overlayVisible = false;
    window.addEventListener('keydown', (e: KeyboardEvent) => {
      if (e.code === 'Backquote') {
        overlayVisible = !overlayVisible;
        overlay?.setVisible(overlayVisible);
        console.log(`[overlay] visible=${overlayVisible}`);
      }
    });

    // -----------------------------------------------------------------------
    // Car mesh (placeholder cube)
    // -----------------------------------------------------------------------
    const cubeGeometry = new THREE.BoxGeometry(1, 0.5, 2);
    const cubeMaterial = new THREE.MeshStandardMaterial({ color: 0x44aa88 });
    const cube = new THREE.Mesh(cubeGeometry, cubeMaterial);
    cube.position.y = 0.25;
    scene.add(cube);

    const _pos = new THREE.Vector3();
    const _quat = new THREE.Quaternion();

    function onResize(): void {
      camera.aspect = window.innerWidth / window.innerHeight;
      camera.updateProjectionMatrix();
      renderer.setSize(window.innerWidth, window.innerHeight);
    }

    window.addEventListener('resize', onResize);

    function animate(): void {
      requestAnimationFrame(animate);

      const ps = predictor.renderState;
      _pos.set(ps.position[0], ps.position[1], ps.position[2]);

      if (_pos.y < 0.25) _pos.y = 0.25;

      cube.position.copy(_pos);
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
