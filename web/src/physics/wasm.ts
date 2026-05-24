/**
 * web/src/physics/wasm.ts — WASM loader and JS bridge for internal/physics.
 *
 * Exposes:
 *   loadPhysics(wasmUrl, wasmExecUrl): Promise<Physics>
 *
 * The returned Physics object mirrors the Go simPhysics global registered
 * by cmd/physicswasm/main.go.
 *
 * # Buffer layouts (all values little-endian)
 *
 * stateBuf (96 bytes) — matches SYNC NOTE in cmd/physicswasm/main.go:
 *   [  0: 12]  Position    float32[3]
 *   [ 12: 28]  Orientation float32[4]  (quaternion x,y,z,w)
 *   [ 28: 40]  LinearVel   float32[3]
 *   [ 40: 52]  AngularVel  float32[3]
 *   [ 52: 56]  RPM         float32
 *   [ 56]      Gear        int8 (as uint8)
 *   [ 57]      padding
 *   [ 58: 74]  WheelLoad   float32[4]
 *   [ 74: 90]  WheelSlip   float32[4]
 *   [ 90]      Grounded    uint8
 *   [ 91: 96]  padding
 *
 * inputBuf (20 bytes) — matches fixture record layout:
 *   [  0:  4]  Seq       uint32  (pass 0; ignored by Step)
 *   [  4:  8]  Throttle  float32
 *   [  8: 12]  Brake     float32
 *   [ 12: 16]  Steer     float32
 *   [ 16]      Gear      int8 (as uint8)
 *   [ 17]      Handbrake uint8
 *   [ 18: 20]  padding
 *
 * SYNC NOTE: stateBuf field order MUST mirror hashStateStream in
 * internal/physics/replay_test.go and decodeState/encodeState in
 * cmd/physicswasm/main.go. If the Go field order changes, update this
 * file too.
 */

/** Raw vehicle state buffer (96 bytes). */
export type StateBuf = Uint8Array;

/** Raw driver input buffer (20 bytes). */
export type InputBuf = Uint8Array;

/** Raw constants buffer (128 bytes). */
export type ConstsBuf = Uint8Array;

/** Decoded vehicle state mirroring internal/physics.State. */
export interface PhysicsState {
  position: [number, number, number];
  orientation: [number, number, number, number];
  linearVel: [number, number, number];
  angularVel: [number, number, number];
  rpm: number;
  gear: number;
  wheelLoad: [number, number, number, number];
  wheelSlip: [number, number, number, number];
  grounded: number;
}

/** Decoded vehicle constants mirroring internal/physics.Constants. */
export interface PhysicsConstants {
  mass: number;
  wheelbase: number;
  trackWidth: number;
  maxEngineTorque: number;
  dragCoeff: number;
  downforceCoeff: number;
  maxSteerAngle: number;
  finalDrive: number;
  drivetrainEfficiency: number;
  wheelRadius: number;
  rollingResistCoeff: number;
  frontalArea: number;
  airDensity: number;
  maxBrakeForce: number;
  idleRPM: number;
  redlineRPM: number;
  pacejkaB: number;
  pacejkaC: number;
  pacejkaD: number;
  yawInertia: number;
  weightDistributionFront: number;
  cgHeight: number;
  rollStiffnessFront: number;
  gearRatios: number[];
}

/** Public interface returned by loadPhysics. */
export interface Physics {
  /**
   * Advance the vehicle simulation by dt seconds.
   * @param stateBuf 96-byte encoded current state.
   * @param inputBuf 20-byte encoded driver input.
   * @param dt       Time step in seconds (typically 1/60).
   * @returns        96-byte encoded next state.
   */
  step(stateBuf: StateBuf, inputBuf: InputBuf, dt: number): StateBuf;

  /**
   * Returns the default vehicle constants as a decoded object.
   */
  defaultConstants(): PhysicsConstants;
}

/** The Go-registered global set by cmd/physicswasm/main.go. */
interface SimPhysicsGlobal {
  step(stateBuf: Uint8Array, inputBuf: Uint8Array, dt: number): Uint8Array;
  defaultConstants(constsBuf?: Uint8Array): Uint8Array;
}

declare global {
  // eslint-disable-next-line no-var
  var simPhysics: SimPhysicsGlobal | undefined;
}

/**
 * Encode a PhysicsState into a 96-byte Uint8Array.
 * Field order matches SYNC NOTE above.
 */
export function encodeState(s: PhysicsState): StateBuf {
  const buf = new Uint8Array(96);
  const view = new DataView(buf.buffer);
  view.setFloat32(0, s.position[0], true);
  view.setFloat32(4, s.position[1], true);
  view.setFloat32(8, s.position[2], true);
  view.setFloat32(12, s.orientation[0], true);
  view.setFloat32(16, s.orientation[1], true);
  view.setFloat32(20, s.orientation[2], true);
  view.setFloat32(24, s.orientation[3], true);
  view.setFloat32(28, s.linearVel[0], true);
  view.setFloat32(32, s.linearVel[1], true);
  view.setFloat32(36, s.linearVel[2], true);
  view.setFloat32(40, s.angularVel[0], true);
  view.setFloat32(44, s.angularVel[1], true);
  view.setFloat32(48, s.angularVel[2], true);
  view.setFloat32(52, s.rpm, true);
  view.setUint8(56, s.gear & 0xff);
  // byte 57 = padding
  view.setFloat32(58, s.wheelLoad[0], true);
  view.setFloat32(62, s.wheelLoad[1], true);
  view.setFloat32(66, s.wheelLoad[2], true);
  view.setFloat32(70, s.wheelLoad[3], true);
  view.setFloat32(74, s.wheelSlip[0], true);
  view.setFloat32(78, s.wheelSlip[1], true);
  view.setFloat32(82, s.wheelSlip[2], true);
  view.setFloat32(86, s.wheelSlip[3], true);
  view.setUint8(90, s.grounded);
  return buf;
}

/**
 * Decode a 96-byte Uint8Array into a PhysicsState.
 * Field order matches SYNC NOTE above.
 */
export function decodeState(buf: StateBuf): PhysicsState {
  const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
  return {
    position: [
      view.getFloat32(0, true),
      view.getFloat32(4, true),
      view.getFloat32(8, true),
    ],
    orientation: [
      view.getFloat32(12, true),
      view.getFloat32(16, true),
      view.getFloat32(20, true),
      view.getFloat32(24, true),
    ],
    linearVel: [
      view.getFloat32(28, true),
      view.getFloat32(32, true),
      view.getFloat32(36, true),
    ],
    angularVel: [
      view.getFloat32(40, true),
      view.getFloat32(44, true),
      view.getFloat32(48, true),
    ],
    rpm: view.getFloat32(52, true),
    gear: view.getInt8(56),
    wheelLoad: [
      view.getFloat32(58, true),
      view.getFloat32(62, true),
      view.getFloat32(66, true),
      view.getFloat32(70, true),
    ],
    wheelSlip: [
      view.getFloat32(74, true),
      view.getFloat32(78, true),
      view.getFloat32(82, true),
      view.getFloat32(86, true),
    ],
    grounded: view.getUint8(90),
  };
}

/**
 * Encode a driver input into a 20-byte Uint8Array.
 */
export function encodeInput(opts: {
  throttle: number;
  brake: number;
  steer: number;
  gear: number;
  handbrake: boolean;
  seq?: number;
}): InputBuf {
  const buf = new Uint8Array(20);
  const view = new DataView(buf.buffer);
  view.setUint32(0, opts.seq ?? 0, true);
  view.setFloat32(4, opts.throttle, true);
  view.setFloat32(8, opts.brake, true);
  view.setFloat32(12, opts.steer, true);
  view.setUint8(16, opts.gear & 0xff);
  view.setUint8(17, opts.handbrake ? 1 : 0);
  return buf;
}

/**
 * Decode a 128-byte constants buffer into a PhysicsConstants object.
 */
function decodeConstants(buf: ConstsBuf): PhysicsConstants {
  const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
  const gearCount = view.getUint32(92, true);
  const gearRatios: number[] = [];
  for (let i = 0; i < Math.min(gearCount, 8); i++) {
    gearRatios.push(view.getFloat32(96 + i * 4, true));
  }
  return {
    mass: view.getFloat32(0, true),
    wheelbase: view.getFloat32(4, true),
    trackWidth: view.getFloat32(8, true),
    maxEngineTorque: view.getFloat32(12, true),
    dragCoeff: view.getFloat32(16, true),
    downforceCoeff: view.getFloat32(20, true),
    maxSteerAngle: view.getFloat32(24, true),
    finalDrive: view.getFloat32(28, true),
    drivetrainEfficiency: view.getFloat32(32, true),
    wheelRadius: view.getFloat32(36, true),
    rollingResistCoeff: view.getFloat32(40, true),
    frontalArea: view.getFloat32(44, true),
    airDensity: view.getFloat32(48, true),
    maxBrakeForce: view.getFloat32(52, true),
    idleRPM: view.getFloat32(56, true),
    redlineRPM: view.getFloat32(60, true),
    pacejkaB: view.getFloat32(64, true),
    pacejkaC: view.getFloat32(68, true),
    pacejkaD: view.getFloat32(72, true),
    yawInertia: view.getFloat32(76, true),
    weightDistributionFront: view.getFloat32(80, true),
    cgHeight: view.getFloat32(84, true),
    rollStiffnessFront: view.getFloat32(88, true),
    gearRatios,
  };
}

/**
 * Load the physics WASM module and return the Physics bridge.
 *
 * @param wasmUrl     URL to physics.wasm (e.g. '/physics.wasm').
 * @param wasmExecUrl URL to wasm_exec.js (e.g. '/wasm_exec.js').
 */
export async function loadPhysics(
  wasmUrl: string,
  wasmExecUrl: string,
): Promise<Physics> {
  // Load the Go WASM runtime shim.
  await loadScript(wasmExecUrl);

  // Instantiate the Go runtime.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const go = new (globalThis as any).Go();

  const result = await WebAssembly.instantiateStreaming(fetch(wasmUrl), go.importObject);
  go.run(result.instance);

  // Wait until the WASM module registers simPhysics.
  await waitFor(() => globalThis.simPhysics !== undefined, 5000);

  const sim = globalThis.simPhysics!;

  return {
    step(stateBuf: StateBuf, inputBuf: InputBuf, dt: number): StateBuf {
      return sim.step(stateBuf, inputBuf, dt);
    },
    defaultConstants(): PhysicsConstants {
      const constsBuf = sim.defaultConstants();
      return decodeConstants(constsBuf);
    },
  };
}

/** Dynamically load an external script and wait for it to execute. */
function loadScript(url: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = url;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error(`Failed to load script: ${url}`));
    document.head.appendChild(script);
  });
}

/** Poll until predicate returns true or timeout elapses. */
function waitFor(predicate: () => boolean, timeoutMs: number): Promise<void> {
  return new Promise((resolve, reject) => {
    const start = Date.now();
    const interval = setInterval(() => {
      if (predicate()) {
        clearInterval(interval);
        resolve();
      } else if (Date.now() - start > timeoutMs) {
        clearInterval(interval);
        reject(new Error('Timed out waiting for simPhysics to register'));
      }
    }, 10);
  });
}
