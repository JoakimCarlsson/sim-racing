/**
 * web/src/physics/parity.test.ts — Go↔WASM parity test.
 *
 * Replays internal/physics/testdata/lap_inputs.bin through the compiled
 * physics.wasm and asserts that the resulting state-stream SHA-256 hash
 * matches the committed golden digest.
 *
 * This test runs under `bun test` which provides a Node-compatible runtime
 * (no browser APIs). It reads the fixture and WASM binary from disk via the
 * Node `fs` module and uses WebAssembly.instantiate (not instantiateStreaming).
 *
 * # Parity contract
 *
 * The WASM binary is compiled from the SAME Go source as the server (standard
 * Go, not TinyGo). The hash produced here MUST equal the golden produced by
 * `go test -run TestReplayDeterminism ./internal/physics/` on linux/amd64.
 *
 * Expected golden: 9b4d7906439c46568a1f7cd85f82e5164f89e27aedf383a70bbfd1350ec52e37
 *
 * # Field order (SYNC NOTE)
 *
 * hashStateStream writes fields in this order to SHA-256:
 *   Position[0..2], Orientation[0..3], LinearVel[0..2], AngularVel[0..2],
 *   RPM, Gear (uint8), WheelLoad[0..3], WheelSlip[0..3], Grounded (uint8).
 * Each float32 is written as 4 LE bytes via DataView.setFloat32.
 * Gear and Grounded are written as 1 byte each.
 *
 * This order matches cmd/physicswasm/main.go encodeState/decodeState and
 * hashStateStream in internal/physics/replay_test.go.
 */

import { describe, test, expect, beforeAll } from 'bun:test';
import fs from 'fs';
import path from 'path';
import { createHash } from 'crypto';
import { fileURLToPath } from 'url';

// ---------------------------------------------------------------------------
// Path resolution (works under `bun test` with A6 upheld)
// ---------------------------------------------------------------------------

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const LAP_INPUTS_BIN = path.resolve(
  __dirname,
  '../../../internal/physics/testdata/lap_inputs.bin',
);
const LAP_INPUTS_GOLDEN = path.resolve(
  __dirname,
  '../../../internal/physics/testdata/lap_inputs.golden',
);
const PHYSICS_WASM = path.resolve(__dirname, '../../public/physics.wasm');
const WASM_EXEC_JS = path.resolve(__dirname, '../../public/wasm_exec.js');

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface SimPhysics {
  step(
    stateBuf: Uint8Array,
    inputBuf: Uint8Array,
    dt: number,
  ): Uint8Array;
  defaultConstants(constsBuf?: Uint8Array): Uint8Array;
}

// ---------------------------------------------------------------------------
// WASM bootstrap (bun/Node path — no fetch, no DOM)
// ---------------------------------------------------------------------------

let simPhysics: SimPhysics | null = null;

/**
 * Bootstrap the Go WASM runtime and wait for simPhysics to register.
 * This runs once per test file via beforeAll.
 */
async function bootstrapWasm(): Promise<void> {
  // 1. Load wasm_exec.js into the current Node/bun global scope.
  //    wasm_exec.js is a CommonJS-compatible script that attaches `Go` to
  //    globalThis. Under bun, eval of its source works as a side-effect.
  const wasmExecSrc = fs.readFileSync(WASM_EXEC_JS, 'utf8');
  // eslint-disable-next-line no-eval
  eval(wasmExecSrc);

  // 2. Instantiate the WASM module.
  const wasmBytes = fs.readFileSync(PHYSICS_WASM);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const go = new (globalThis as any).Go();
  const result = await WebAssembly.instantiate(wasmBytes, go.importObject);

  // 3. Run the Go program (registers simPhysics then blocks on channel).
  go.run(result.instance);

  // 4. Poll until simPhysics is registered.
  await new Promise<void>((resolve, reject) => {
    const start = Date.now();
    const interval = setInterval(() => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      if ((globalThis as any).simPhysics !== undefined) {
        clearInterval(interval);
        resolve();
      } else if (Date.now() - start > 5000) {
        clearInterval(interval);
        reject(new Error('Timed out waiting for simPhysics to register'));
      }
    }, 10);
  });

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  simPhysics = (globalThis as any).simPhysics as SimPhysics;
}

// ---------------------------------------------------------------------------
// Fixture parsing (mirrors loadFixture in replay_test.go)
// ---------------------------------------------------------------------------

const FIXTURE_REC_SIZE = 20; // bytes per record

interface InputRecord {
  throttle: number;
  brake: number;
  steer: number;
  gear: number;
  handbrake: boolean;
}

function loadFixture(binPath: string): InputRecord[] {
  const data = fs.readFileSync(binPath);
  if (data.byteLength % FIXTURE_REC_SIZE !== 0) {
    throw new Error(
      `Fixture size ${data.byteLength} is not a multiple of ${FIXTURE_REC_SIZE}`,
    );
  }
  const n = data.byteLength / FIXTURE_REC_SIZE;
  const view = new DataView(
    data.buffer,
    data.byteOffset,
    data.byteLength,
  );
  const records: InputRecord[] = [];
  for (let i = 0; i < n; i++) {
    const off = i * FIXTURE_REC_SIZE;
    records.push({
      // off+0: Seq uint32 (ignored)
      throttle: view.getFloat32(off + 4, true),
      brake: view.getFloat32(off + 8, true),
      steer: view.getFloat32(off + 12, true),
      gear: view.getInt8(off + 16),
      handbrake: data[off + 17] !== 0,
    });
  }
  return records;
}

// ---------------------------------------------------------------------------
// Buffer helpers (mirrors decodeState / encodeInput in cmd/physicswasm)
// ---------------------------------------------------------------------------

/** Create initial state buffer: gear=1, orientation=[0,0,0,1], rest zero. */
function initialStateBuf(): Uint8Array {
  const buf = new Uint8Array(96);
  const view = new DataView(buf.buffer);
  // Position = [0,0,0] (already zero)
  // Orientation = [0,0,0,1] at offsets 12,16,20,24
  view.setFloat32(12, 0, true); // qx
  view.setFloat32(16, 0, true); // qy
  view.setFloat32(20, 0, true); // qz
  view.setFloat32(24, 1, true); // qw
  // RPM = 0 (already zero — will be clamped to idleRPM by Step)
  view.setUint8(56, 1); // Gear = 1
  return buf;
}

/** Encode one InputRecord into a 20-byte inputBuf. */
function encodeInput(r: InputRecord, seq: number): Uint8Array {
  const buf = new Uint8Array(20);
  const view = new DataView(buf.buffer);
  view.setUint32(0, seq, true);
  view.setFloat32(4, r.throttle, true);
  view.setFloat32(8, r.brake, true);
  view.setFloat32(12, r.steer, true);
  view.setUint8(16, r.gear & 0xff);
  view.setUint8(17, r.handbrake ? 1 : 0);
  return buf;
}

// ---------------------------------------------------------------------------
// Hash routine (mirrors hashStateStream in internal/physics/replay_test.go)
//
// Field order: Position[3], Orientation[4], LinearVel[3], AngularVel[3],
//              RPM, Gear(uint8), WheelLoad[4], WheelSlip[4], Grounded(uint8).
// Each float32 → 4 LE bytes; Gear/Grounded → 1 byte each.
// Sampling: every REPLAY_HZ-th tick (every 60 ticks at 60 Hz).
// ---------------------------------------------------------------------------

const REPLAY_HZ = 60;
const DT = 1.0 / REPLAY_HZ;

function hashStateStream(
  records: InputRecord[],
  physics: SimPhysics,
): string {
  const hash = createHash('sha256');
  let stateBuf = initialStateBuf();

  const scratch = new Uint8Array(4);
  const scratchView = new DataView(scratch.buffer);

  const writeF32 = (v: number) => {
    scratchView.setFloat32(0, v, true);
    hash.update(scratch);
  };
  const writeU8 = (v: number) => {
    hash.update(new Uint8Array([v & 0xff]));
  };

  for (let i = 0; i < records.length; i++) {
    const inputBuf = encodeInput(records[i], i);
    stateBuf = physics.step(stateBuf, inputBuf, DT);

    if ((i + 1) % REPLAY_HZ === 0) {
      const view = new DataView(
        stateBuf.buffer,
        stateBuf.byteOffset,
        stateBuf.byteLength,
      );
      writeF32(view.getFloat32(0, true));   // Position[0]
      writeF32(view.getFloat32(4, true));   // Position[1]
      writeF32(view.getFloat32(8, true));   // Position[2]
      writeF32(view.getFloat32(12, true));  // Orientation[0]
      writeF32(view.getFloat32(16, true));  // Orientation[1]
      writeF32(view.getFloat32(20, true));  // Orientation[2]
      writeF32(view.getFloat32(24, true));  // Orientation[3]
      writeF32(view.getFloat32(28, true));  // LinearVel[0]
      writeF32(view.getFloat32(32, true));  // LinearVel[1]
      writeF32(view.getFloat32(36, true));  // LinearVel[2]
      writeF32(view.getFloat32(40, true));  // AngularVel[0]
      writeF32(view.getFloat32(44, true));  // AngularVel[1]
      writeF32(view.getFloat32(48, true));  // AngularVel[2]
      writeF32(view.getFloat32(52, true));  // RPM
      writeU8(view.getUint8(56));           // Gear
      writeF32(view.getFloat32(58, true));  // WheelLoad[0]
      writeF32(view.getFloat32(62, true));  // WheelLoad[1]
      writeF32(view.getFloat32(66, true));  // WheelLoad[2]
      writeF32(view.getFloat32(70, true));  // WheelLoad[3]
      writeF32(view.getFloat32(74, true));  // WheelSlip[0]
      writeF32(view.getFloat32(78, true));  // WheelSlip[1]
      writeF32(view.getFloat32(82, true));  // WheelSlip[2]
      writeF32(view.getFloat32(86, true));  // WheelSlip[3]
      writeU8(view.getUint8(90));           // Grounded
    }
  }

  return hash.digest('hex');
}

// ---------------------------------------------------------------------------
// Decode constants helper (for AC2 assertions)
// ---------------------------------------------------------------------------

function decodeConstantsBuf(buf: Uint8Array): {
  mass: number;
  redlineRPM: number;
  gearRatiosLength: number;
} {
  const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
  return {
    mass: view.getFloat32(0, true),
    redlineRPM: view.getFloat32(60, true),
    gearRatiosLength: view.getUint32(92, true),
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('Go↔WASM parity', () => {
  beforeAll(async () => {
    await bootstrapWasm();
  });

  test('defaultConstants returns mass=1200, redlineRPM=7000, gearRatios.length=8', () => {
    expect(simPhysics).not.toBeNull();
    const constsBuf = simPhysics!.defaultConstants();
    const { mass, redlineRPM, gearRatiosLength } =
      decodeConstantsBuf(constsBuf);
    expect(mass).toBeCloseTo(1200, 0);
    expect(redlineRPM).toBeCloseTo(7000, 0);
    expect(gearRatiosLength).toBe(8);
  });

  test('WASM state-stream hash matches golden over lap_inputs.bin', async () => {
    expect(simPhysics).not.toBeNull();

    const records = loadFixture(LAP_INPUTS_BIN);
    const goldenRaw = fs.readFileSync(LAP_INPUTS_GOLDEN, 'utf8').trim();

    const got = hashStateStream(records, simPhysics!);

    expect(got).toBe(goldenRaw);
  }, 60_000 /* 60 s timeout for 1800 ticks */);
});
