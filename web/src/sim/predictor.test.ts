/**
 * web/src/sim/predictor.test.ts — Predictor unit tests.
 *
 * AC4: Bun test feeds N synthetic inputs through predictor + direct
 *      physics.step; predictedState matches Go-native bit-for-bit (Float32).
 * AC3: inputHistorySize capped at 256 after 1000 ticks.
 * + applySnapshot stores confirmedSeq/State; missing own car = no-op.
 * + applySnapshot does NOT mutate predictedState.
 *
 * WASM bootstrap follows the same pattern as parity.test.ts.
 */

import { describe, test, expect, beforeAll } from 'bun:test';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import { Predictor, MAX_HISTORY } from './predictor';
import { encodeState, decodeState, encodeInput } from '../physics/wasm';
import type { PhysicsState } from '../physics/wasm';
import type { Physics } from '../physics/wasm';
import type { ServerSnapshot } from '../protocol/messages';
import type { PhysicsInput } from '../protocol/messages';

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const PHYSICS_WASM = path.resolve(__dirname, '../../public/physics.wasm');
const WASM_EXEC_JS = path.resolve(__dirname, '../../public/wasm_exec.js');

// ---------------------------------------------------------------------------
// WASM bootstrap (same pattern as parity.test.ts)
// ---------------------------------------------------------------------------

interface SimPhysicsRaw {
  step(stateBuf: Uint8Array, inputBuf: Uint8Array, dt: number): Uint8Array;
  defaultConstants(constsBuf?: Uint8Array): Uint8Array;
}

let rawPhysics: SimPhysicsRaw | null = null;
let physics: Physics | null = null;

async function bootstrapWasm(): Promise<void> {
  const wasmExecSrc = fs.readFileSync(WASM_EXEC_JS, 'utf8');
  // eslint-disable-next-line no-eval
  eval(wasmExecSrc);

  const wasmBytes = fs.readFileSync(PHYSICS_WASM);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const go = new (globalThis as any).Go();
  const result = await WebAssembly.instantiate(wasmBytes, go.importObject);
  go.run(result.instance);

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
  rawPhysics = (globalThis as any).simPhysics as SimPhysicsRaw;

  // Wrap into the Physics interface shape.
  physics = {
    step(stateBuf, inputBuf, dt) {
      return rawPhysics!.step(stateBuf, inputBuf, dt);
    },
    defaultConstants() {
      // Not needed in these tests; returning an empty stub.
      throw new Error('defaultConstants not used in predictor tests');
    },
  };
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeInitialState(): PhysicsState {
  return {
    position: [0, 0, 0],
    orientation: [0, 0, 0, 1],
    linearVel: [0, 0, 0],
    angularVel: [0, 0, 0],
    rpm: 0,
    gear: 1,
    wheelLoad: [0, 0, 0, 0],
    wheelSlip: [0, 0, 0, 0],
    grounded: 0,
  };
}

/** Step directly through WASM N times, returning final PhysicsState. */
function directStep(
  p: Physics,
  initial: PhysicsState,
  inputs: PhysicsInput[],
  dt = 1 / 60,
): PhysicsState {
  let buf = encodeState(initial);
  for (let i = 0; i < inputs.length; i++) {
    const inputBuf = encodeInput({
      seq: i,
      throttle: inputs[i].throttle,
      brake: inputs[i].brake,
      steer: inputs[i].steer,
      gear: inputs[i].gear,
      handbrake: inputs[i].handbrake,
    });
    buf = p.step(buf, inputBuf, dt);
  }
  return decodeState(buf);
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('Predictor', () => {
  beforeAll(async () => {
    await bootstrapWasm();
  });

  // -------------------------------------------------------------------------
  // AC4: parity vs direct physics.step over 120 ticks
  // -------------------------------------------------------------------------
  test('(AC4) predictedState matches direct physics.step bit-for-bit over 120 ticks', () => {
    expect(physics).not.toBeNull();

    const predictor = new Predictor(physics!);

    // Build input list: alternating throttle/steer to stress more state paths.
    const inputs: PhysicsInput[] = [];
    for (let i = 0; i < 120; i++) {
      const input: PhysicsInput = {
        throttle: 1,
        brake: 0,
        steer: i % 2 === 0 ? 0.3 : -0.3,
        gear: 1,
        handbrake: false,
      };
      inputs.push(input);
    }

    // Feed through predictor.
    for (let i = 0; i < 120; i++) {
      predictor.tick(i, inputs[i]);
    }

    // Feed through direct WASM step.
    const direct = directStep(physics!, makeInitialState(), inputs);
    const predicted = predictor.predictedState;

    // Float32 bit-for-bit equality on all fields.
    expect(predicted.position[0]).toBe(direct.position[0]);
    expect(predicted.position[1]).toBe(direct.position[1]);
    expect(predicted.position[2]).toBe(direct.position[2]);

    expect(predicted.orientation[0]).toBe(direct.orientation[0]);
    expect(predicted.orientation[1]).toBe(direct.orientation[1]);
    expect(predicted.orientation[2]).toBe(direct.orientation[2]);
    expect(predicted.orientation[3]).toBe(direct.orientation[3]);

    expect(predicted.linearVel[0]).toBe(direct.linearVel[0]);
    expect(predicted.linearVel[1]).toBe(direct.linearVel[1]);
    expect(predicted.linearVel[2]).toBe(direct.linearVel[2]);

    expect(predicted.angularVel[0]).toBe(direct.angularVel[0]);
    expect(predicted.angularVel[1]).toBe(direct.angularVel[1]);
    expect(predicted.angularVel[2]).toBe(direct.angularVel[2]);

    expect(predicted.rpm).toBe(direct.rpm);
    expect(predicted.gear).toBe(direct.gear);

    expect(predicted.wheelLoad[0]).toBe(direct.wheelLoad[0]);
    expect(predicted.wheelLoad[1]).toBe(direct.wheelLoad[1]);
    expect(predicted.wheelLoad[2]).toBe(direct.wheelLoad[2]);
    expect(predicted.wheelLoad[3]).toBe(direct.wheelLoad[3]);

    expect(predicted.wheelSlip[0]).toBe(direct.wheelSlip[0]);
    expect(predicted.wheelSlip[1]).toBe(direct.wheelSlip[1]);
    expect(predicted.wheelSlip[2]).toBe(direct.wheelSlip[2]);
    expect(predicted.wheelSlip[3]).toBe(direct.wheelSlip[3]);

    expect(predicted.grounded).toBe(direct.grounded);
  }, 30_000);

  // -------------------------------------------------------------------------
  // AC3: inputHistorySize capped at 256 after 1000 ticks
  // -------------------------------------------------------------------------
  test('(AC3) inputHistorySize === MAX_HISTORY (256) after 1000 ticks', () => {
    expect(physics).not.toBeNull();

    const predictor = new Predictor(physics!);
    const input: PhysicsInput = { throttle: 0, brake: 0, steer: 0, gear: 1, handbrake: false };

    for (let i = 0; i < 1000; i++) {
      predictor.tick(i, input);
    }

    expect(predictor.inputHistorySize()).toBe(MAX_HISTORY);
    expect(MAX_HISTORY).toBe(256);
  }, 30_000);

  // -------------------------------------------------------------------------
  // applySnapshot: stores confirmedSeq/State; missing own car = no-op
  // -------------------------------------------------------------------------
  test('applySnapshot stores confirmedSeq and confirmedState for own car', () => {
    expect(physics).not.toBeNull();

    const predictor = new Predictor(physics!);

    expect(predictor.confirmedState).toBeNull();
    expect(predictor.confirmedSeq).toBeNull();

    const snap: ServerSnapshot = {
      serverTickMs: 1000,
      lastAckedSeq: 42,
      cars: [
        {
          playerID: 7,
          nickname: 'tester',
          state: {
            position: [1, 2, 3],
            orientation: [0, 0, 0, 1],
            linearVel: [4, 5, 6],
            angularVel: [0, 0, 0],
            rpm: 800,
            gear: 2,
            wheelLoad: [100, 100, 100, 100],
            wheelSlip: [0.1, 0.1, 0.1, 0.1],
            grounded: 0b1111,
          },
        },
      ],
    };

    predictor.applySnapshot(snap, 7);

    expect(predictor.confirmedSeq).toBe(42);
    expect(predictor.confirmedState).not.toBeNull();
    expect(predictor.confirmedState!.position[0]).toBe(1);
    expect(predictor.confirmedState!.position[1]).toBe(2);
    expect(predictor.confirmedState!.position[2]).toBe(3);
    expect(predictor.confirmedState!.rpm).toBe(800);
    expect(predictor.confirmedState!.gear).toBe(2);
  });

  test('applySnapshot is a no-op when own car is not in snapshot', () => {
    expect(physics).not.toBeNull();

    const predictor = new Predictor(physics!);

    const snap: ServerSnapshot = {
      serverTickMs: 2000,
      lastAckedSeq: 99,
      cars: [
        {
          playerID: 3, // different player
          nickname: 'other',
          state: {
            position: [10, 20, 30],
            orientation: [0, 0, 0, 1],
            linearVel: [0, 0, 0],
            angularVel: [0, 0, 0],
            rpm: 0,
            gear: 1,
            wheelLoad: [0, 0, 0, 0],
            wheelSlip: [0, 0, 0, 0],
            grounded: 0,
          },
        },
      ],
    };

    predictor.applySnapshot(snap, 7); // ownPlayerID=7 not in cars

    expect(predictor.confirmedSeq).toBeNull();
    expect(predictor.confirmedState).toBeNull();
  });

  // -------------------------------------------------------------------------
  // applySnapshot does NOT mutate predictedState
  // -------------------------------------------------------------------------
  test('applySnapshot does NOT mutate predictedState', () => {
    expect(physics).not.toBeNull();

    const predictor = new Predictor(physics!);

    // Run a few ticks to get a non-zero predicted state.
    const input: PhysicsInput = { throttle: 1, brake: 0, steer: 0, gear: 1, handbrake: false };
    for (let i = 0; i < 10; i++) {
      predictor.tick(i, input);
    }

    // Capture the predicted state before snapshot.
    const beforePos = [...predictor.predictedState.position] as [number, number, number];
    const beforeRpm = predictor.predictedState.rpm;

    // Apply a snapshot with a very different position.
    const snap: ServerSnapshot = {
      serverTickMs: 500,
      lastAckedSeq: 5,
      cars: [
        {
          playerID: 1,
          nickname: 'me',
          state: {
            position: [999, 999, 999],
            orientation: [0, 0, 0, 1],
            linearVel: [0, 0, 0],
            angularVel: [0, 0, 0],
            rpm: 0,
            gear: 1,
            wheelLoad: [0, 0, 0, 0],
            wheelSlip: [0, 0, 0, 0],
            grounded: 0,
          },
        },
      ],
    };

    predictor.applySnapshot(snap, 1);

    // predictedState must remain unchanged.
    expect(predictor.predictedState.position[0]).toBe(beforePos[0]);
    expect(predictor.predictedState.position[1]).toBe(beforePos[1]);
    expect(predictor.predictedState.position[2]).toBe(beforePos[2]);
    expect(predictor.predictedState.rpm).toBe(beforeRpm);

    // confirmedState should reflect the snapshot's car state.
    expect(predictor.confirmedState!.position[0]).toBe(999);
  });
});

