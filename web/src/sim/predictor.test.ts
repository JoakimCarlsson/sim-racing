/**
 * web/src/sim/predictor.test.ts — Predictor unit tests.
 *
 * AC1: Reconcile in <1ms for ≤32 inputs (median of 50 runs via performance.now()).
 * AC2: Drive 20 ticks throttle=1; synthetic confirmedState (offset +5m on X)
 *      with lastAckedSeq=10; expected = directStep(confirmedState, inputs[11..19]);
 *      assert predictedState bit-equal.
 * AC3: Visual smoothing — <0.5m pos AND <5° yaw delta → ease over ~100ms;
 *      otherwise snap (smoothingOffset !== [0,0,0] vs [0,0,0]).
 * AC4: predictionError accessor (null pre-snapshot, {positionDelta, yawDeltaRad} post).
 * + (old AC3/AC4 now renumbered) inputHistorySize capped at 256 after 1000 ticks.
 * + applySnapshot stores confirmedSeq/State; missing own car = no-op.
 * + applySnapshot (NEW contract): with reconciliation + zero unacked history,
 *   predictedState equals confirmedState (replay of empty history).
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

/** Build a ServerSnapshot for a given player. */
function makeSnap(playerID: number, lastAckedSeq: number, state: PhysicsState): ServerSnapshot {
  return {
    serverTickMs: 1000,
    lastAckedSeq,
    cars: [
      {
        playerID,
        nickname: 'tester',
        state: {
          position: state.position,
          orientation: state.orientation,
          linearVel: state.linearVel,
          angularVel: state.angularVel,
          rpm: state.rpm,
          gear: state.gear,
          wheelLoad: state.wheelLoad,
          wheelSlip: state.wheelSlip,
          grounded: state.grounded,
        },
      },
    ],
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('Predictor', () => {
  beforeAll(async () => {
    await bootstrapWasm();
  });

  // -------------------------------------------------------------------------
  // Parity: predictedState matches direct physics.step over 120 ticks
  // -------------------------------------------------------------------------
  test('(parity) predictedState matches direct physics.step bit-for-bit over 120 ticks', () => {
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
  // History cap: inputHistorySize capped at 256 after 1000 ticks
  // -------------------------------------------------------------------------
  test('(history-cap) inputHistorySize === MAX_HISTORY (256) after 1000 ticks', () => {
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
  // applySnapshot NEW contract: with reconcile + zero history, predictedState
  // equals confirmedState (replay of empty history is a no-op).
  // NOTE: old test asserted predictedState was NOT mutated; the new contract
  // is that applySnapshot DOES reconcile, so with zero unacked inputs after the
  // confirmed seq, predictedState == confirmedState.
  // -------------------------------------------------------------------------
  test('applySnapshot with zero unacked history sets predictedState = confirmedState', () => {
    expect(physics).not.toBeNull();

    const predictor = new Predictor(physics!);

    // Run ticks 0..9 (seq 0–9). Then snapshot acks seq=9 with an offset state.
    // After reconciliation there are no inputs with seq > 9, so predictedState
    // should equal the confirmed state directly.
    const input: PhysicsInput = { throttle: 1, brake: 0, steer: 0, gear: 1, handbrake: false };
    for (let i = 0; i < 10; i++) {
      predictor.tick(i, input);
    }

    const confirmedPos: [number, number, number] = [999, 0, 0];
    const snap = makeSnap(1, 9, {
      position: confirmedPos,
      orientation: [0, 0, 0, 1],
      linearVel: [0, 0, 0],
      angularVel: [0, 0, 0],
      rpm: 0,
      gear: 1,
      wheelLoad: [0, 0, 0, 0],
      wheelSlip: [0, 0, 0, 0],
      grounded: 0,
    });

    predictor.applySnapshot(snap, 1);

    // No unacked inputs remain (all seq 0..9 are <= lastAckedSeq=9),
    // so replay of confirmedState with 0 inputs = confirmedState itself.
    expect(predictor.predictedState.position[0]).toBe(confirmedPos[0]);
    expect(predictor.predictedState.position[1]).toBe(confirmedPos[1]);
    expect(predictor.predictedState.position[2]).toBe(confirmedPos[2]);

    // confirmedState should also reflect the snapshot's car state.
    expect(predictor.confirmedState!.position[0]).toBe(999);
  });

  // -------------------------------------------------------------------------
  // AC1: Reconcile in <1ms for ≤32 inputs (median of 50 runs via performance.now())
  // -------------------------------------------------------------------------
  test('(AC1) reconciliation completes in <1ms median for 32 unacked inputs', () => {
    expect(physics).not.toBeNull();

    const RUNS = 50;
    const durations: number[] = [];

    for (let run = 0; run < RUNS; run++) {
      const predictor = new Predictor(physics!);
      const input: PhysicsInput = { throttle: 1, brake: 0, steer: 0, gear: 1, handbrake: false };

      // Feed 42 ticks total: first 10 will be acked, leaving 32 unacked.
      for (let i = 0; i < 42; i++) {
        predictor.tick(i, input);
      }

      // Snapshot acking seq=9 (leaves inputs 10..41 = 32 unacked).
      const confirmedState = makeInitialState();
      const snap = makeSnap(1, 9, confirmedState);

      const t0 = performance.now();
      predictor.applySnapshot(snap, 1);
      const t1 = performance.now();
      durations.push(t1 - t0);
    }

    durations.sort((a, b) => a - b);
    const median = durations[Math.floor(RUNS / 2)];

    // Median must be below 1ms.
    expect(median).toBeLessThan(1);
  }, 60_000);

  // -------------------------------------------------------------------------
  // AC2: Drive 20 ticks throttle=1; synthetic confirmedState (offset +5m on X)
  //      with lastAckedSeq=10; expected = directStep(confirmedState, inputs[11..19]);
  //      assert predictedState bit-equal; history compacted to 9 entries.
  // -------------------------------------------------------------------------
  test('(AC2) reconciliation produces bit-equal result to direct replay', () => {
    expect(physics).not.toBeNull();

    const predictor = new Predictor(physics!);

    // 20 inputs, all throttle=1.
    const inputs: PhysicsInput[] = [];
    for (let i = 0; i < 20; i++) {
      inputs.push({ throttle: 1, brake: 0, steer: 0, gear: 1, handbrake: false });
    }

    // Feed all 20 ticks (seq 0..19) through predictor.
    for (let i = 0; i < 20; i++) {
      predictor.tick(i, inputs[i]);
    }

    // Build synthetic confirmedState: same as initial but +5m on X.
    const confirmedState: PhysicsState = {
      ...makeInitialState(),
      position: [5, 0, 0],
    };

    // Snapshot acks seq=10; server's state is confirmedState at that point.
    const snap = makeSnap(1, 10, confirmedState);
    predictor.applySnapshot(snap, 1);

    // Expected: directStep from confirmedState using inputs[11..19] (seqs 11–19).
    // The directStep helper uses index-based seq (0..N-1) for the input buffer
    // but seq in the buffer is ignored by WASM physics.Step; only the actual
    // control values matter. Use inputs[11..19] (9 inputs).
    const unackedInputs = inputs.slice(11); // inputs at seqs 11..19
    const expected = directStep(physics!, confirmedState, unackedInputs);

    const predicted = predictor.predictedState;

    // Bit-equal comparison on all fields.
    expect(predicted.position[0]).toBe(expected.position[0]);
    expect(predicted.position[1]).toBe(expected.position[1]);
    expect(predicted.position[2]).toBe(expected.position[2]);

    expect(predicted.orientation[0]).toBe(expected.orientation[0]);
    expect(predicted.orientation[1]).toBe(expected.orientation[1]);
    expect(predicted.orientation[2]).toBe(expected.orientation[2]);
    expect(predicted.orientation[3]).toBe(expected.orientation[3]);

    expect(predicted.linearVel[0]).toBe(expected.linearVel[0]);
    expect(predicted.linearVel[1]).toBe(expected.linearVel[1]);
    expect(predicted.linearVel[2]).toBe(expected.linearVel[2]);

    expect(predicted.angularVel[0]).toBe(expected.angularVel[0]);
    expect(predicted.angularVel[1]).toBe(expected.angularVel[1]);
    expect(predicted.angularVel[2]).toBe(expected.angularVel[2]);

    expect(predicted.rpm).toBe(expected.rpm);
    expect(predicted.gear).toBe(expected.gear);

    expect(predicted.wheelLoad[0]).toBe(expected.wheelLoad[0]);
    expect(predicted.wheelLoad[1]).toBe(expected.wheelLoad[1]);
    expect(predicted.wheelLoad[2]).toBe(expected.wheelLoad[2]);
    expect(predicted.wheelLoad[3]).toBe(expected.wheelLoad[3]);

    expect(predicted.wheelSlip[0]).toBe(expected.wheelSlip[0]);
    expect(predicted.wheelSlip[1]).toBe(expected.wheelSlip[1]);
    expect(predicted.wheelSlip[2]).toBe(expected.wheelSlip[2]);
    expect(predicted.wheelSlip[3]).toBe(expected.wheelSlip[3]);

    expect(predicted.grounded).toBe(expected.grounded);

    // History compacted: only 9 unacked entries remain (seqs 11..19).
    expect(predictor.inputHistorySize()).toBe(9);
  }, 30_000);

  // -------------------------------------------------------------------------
  // AC3: Visual smoothing
  // -------------------------------------------------------------------------
  test('(AC3-smooth) small error → renderState differs from predictedState immediately after snapshot', () => {
    expect(physics).not.toBeNull();

    const predictor = new Predictor(physics!);
    const input: PhysicsInput = { throttle: 0, brake: 0, steer: 0, gear: 1, handbrake: false };

    // Drive 10 ticks from zero state so we have a non-trivial predicted state.
    for (let i = 0; i < 10; i++) {
      predictor.tick(i, input);
    }

    // Snapshot with a tiny position offset (0.1m < 0.5m threshold) and same yaw.
    // lastAckedSeq=9 so no unacked inputs remain.
    const predictedPosBeforeX = predictor.predictedState.position[0];
    const confirmedState: PhysicsState = {
      ...makeInitialState(),
      // Apply a sub-threshold position shift relative to current predictedState.
      position: [predictedPosBeforeX + 0.1, 0, 0],
    };
    const snap = makeSnap(1, 9, confirmedState);
    predictor.applySnapshot(snap, 1);

    // renderState should still be near the OLD predicted position (smoothed).
    // predictedState is at confirmedPos (empty replay).
    const rs = predictor.renderState;
    const ps = predictor.predictedState;

    // renderState differs from predictedState (offset is active).
    // The offset should be close to -0.1 on X (old was at ~predictedPosBeforeX,
    // new is at confirmedState.position[0] = predictedPosBeforeX + 0.1).
    const diffX = rs.position[0] - ps.position[0];
    // diffX should be approximately -0.1 (old - new offset).
    expect(Math.abs(diffX)).toBeGreaterThan(0.05);
    expect(Math.abs(diffX)).toBeLessThan(0.15);
  }, 30_000);

  test('(AC3-snap) large error → renderState equals predictedState (snap)', () => {
    expect(physics).not.toBeNull();

    const predictor = new Predictor(physics!);
    const input: PhysicsInput = { throttle: 0, brake: 0, steer: 0, gear: 1, handbrake: false };

    for (let i = 0; i < 10; i++) {
      predictor.tick(i, input);
    }

    // Snapshot with large position offset (100m >> 0.5m threshold).
    const confirmedState: PhysicsState = {
      ...makeInitialState(),
      position: [100, 0, 0],
    };
    const snap = makeSnap(1, 9, confirmedState);
    predictor.applySnapshot(snap, 1);

    // After snap: renderState === predictedState (no offset).
    const rs = predictor.renderState;
    const ps = predictor.predictedState;
    expect(rs.position[0]).toBe(ps.position[0]);
    expect(rs.position[1]).toBe(ps.position[1]);
    expect(rs.position[2]).toBe(ps.position[2]);
  }, 30_000);

  test('(AC3-decay) smoothing offset decays after ticks', () => {
    expect(physics).not.toBeNull();

    const predictor = new Predictor(physics!);
    const input: PhysicsInput = { throttle: 0, brake: 0, steer: 0, gear: 1, handbrake: false };

    for (let i = 0; i < 10; i++) {
      predictor.tick(i, input);
    }

    // Small offset snapshot.
    const predictedPosX = predictor.predictedState.position[0];
    const confirmedState: PhysicsState = {
      ...makeInitialState(),
      position: [predictedPosX + 0.1, 0, 0],
    };
    const snap = makeSnap(1, 9, confirmedState);
    predictor.applySnapshot(snap, 1);

    // Capture initial render offset.
    const offsetBefore = predictor.renderState.position[0] - predictor.predictedState.position[0];
    expect(Math.abs(offsetBefore)).toBeGreaterThan(0.05);

    // Run several more ticks (dt=1/60 each) to let offset decay.
    const inputAfter: PhysicsInput = { throttle: 0, brake: 0, steer: 0, gear: 1, handbrake: false };
    // ~20 ticks @ 1/60s = ~333ms >> 100ms τ → offset should be near zero.
    for (let i = 10; i < 30; i++) {
      predictor.tick(i, inputAfter);
    }

    const offsetAfter = predictor.renderState.position[0] - predictor.predictedState.position[0];
    // After 20 ticks the offset should be much smaller than before.
    expect(Math.abs(offsetAfter)).toBeLessThan(Math.abs(offsetBefore) * 0.1);
  }, 30_000);

  // -------------------------------------------------------------------------
  // AC4: predictionError accessor
  // -------------------------------------------------------------------------
  test('(AC4) predictionError is null before first snapshot', () => {
    expect(physics).not.toBeNull();

    const predictor = new Predictor(physics!);
    expect(predictor.predictionError).toBeNull();

    const input: PhysicsInput = { throttle: 1, brake: 0, steer: 0, gear: 1, handbrake: false };
    for (let i = 0; i < 5; i++) {
      predictor.tick(i, input);
    }

    // Still null — no snapshot yet.
    expect(predictor.predictionError).toBeNull();
  }, 30_000);

  test('(AC4) predictionError is populated after applySnapshot', () => {
    expect(physics).not.toBeNull();

    const predictor = new Predictor(physics!);
    const input: PhysicsInput = { throttle: 1, brake: 0, steer: 0, gear: 1, handbrake: false };

    for (let i = 0; i < 10; i++) {
      predictor.tick(i, input);
    }

    // Apply a snapshot that diverges from current prediction.
    const confirmedState: PhysicsState = {
      ...makeInitialState(),
      position: [10, 0, 0], // clearly different from predicted
    };
    const snap = makeSnap(1, 9, confirmedState);
    predictor.applySnapshot(snap, 1);

    const err = predictor.predictionError;
    expect(err).not.toBeNull();
    expect(typeof err!.positionDelta).toBe('number');
    expect(typeof err!.yawDeltaRad).toBe('number');
    // positionDelta should be > 0 because positions differ.
    expect(err!.positionDelta).toBeGreaterThan(0);
  }, 30_000);
});
