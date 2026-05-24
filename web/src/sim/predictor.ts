/**
 * web/src/sim/predictor.ts — Client-side prediction via WASM physics.
 *
 * The Predictor maintains a rolling window of at most MAX_HISTORY=256 unacked
 * input frames. On each tick() it advances the predicted state one step forward
 * using the compiled WASM physics (bit-for-bit identical to the server).
 *
 * applySnapshot() stores the authoritative confirmed state from the server but
 * does NOT rewind / re-simulate (reconciliation is a non-goal for this issue).
 *
 * # Thread safety
 * All methods run on the JS main thread. No locks needed.
 */

import type { Physics, PhysicsState } from '../physics/wasm';
import { encodeState, decodeState, encodeInput } from '../physics/wasm';
import type { PhysicsInput } from '../protocol/messages';
import type { ServerSnapshot } from '../protocol/messages';

/** Maximum number of unacknowledged input frames kept in history. */
export const MAX_HISTORY = 256;

/** An entry in the input history ring buffer. */
interface HistoryEntry {
  seq: number;
  input: PhysicsInput;
}

/** Initial state: orientation=[0,0,0,1], gear=1, everything else zero. */
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

export class Predictor {
  /** The latest predicted vehicle state. Updated by every tick(). */
  private _predictedState: PhysicsState;

  /**
   * The last confirmed state received from the server, or null before the
   * first snapshot arrives.
   */
  private _confirmedState: PhysicsState | null = null;

  /** The seq that matches _confirmedState, or null. */
  private _confirmedSeq: number | null = null;

  /**
   * Ring buffer of sent-but-unacked inputs.
   * Length capped at MAX_HISTORY. When full, the oldest entry is evicted.
   */
  private readonly history: HistoryEntry[] = [];

  /** Index into history[] pointing at the next write slot (ring). */
  private historyHead = 0;

  /** How many entries are currently valid in history[]. */
  private historyCount = 0;

  constructor(private readonly physics: Physics) {
    this._predictedState = makeInitialState();
    // Pre-allocate the ring so push is allocation-free.
    for (let i = 0; i < MAX_HISTORY; i++) {
      this.history.push({ seq: 0, input: { throttle: 0, brake: 0, steer: 0, gear: 1, handbrake: false } });
    }
  }

  // ---------------------------------------------------------------------------
  // Public read-only accessors
  // ---------------------------------------------------------------------------

  /** The latest client-predicted vehicle state. */
  get predictedState(): Readonly<PhysicsState> {
    return this._predictedState;
  }

  /** Last confirmed state from server, or null before first snapshot. */
  get confirmedState(): Readonly<PhysicsState> | null {
    return this._confirmedState;
  }

  /** Seq number matching confirmedState, or null. */
  get confirmedSeq(): number | null {
    return this._confirmedSeq;
  }

  // ---------------------------------------------------------------------------
  // tick — advance prediction by one physics step
  // ---------------------------------------------------------------------------

  /**
   * Advance the predicted state by one physics step.
   *
   * Called by InputSender's onTick callback AFTER a frame is successfully sent,
   * so seq is guaranteed to be the seq that was just sent (no re-use).
   *
   * @param seq   The sequence number of the input just sent.
   * @param input The input that was sent.
   * @param dt    Time step in seconds (default 1/60).
   */
  tick(seq: number, input: PhysicsInput, dt = 1 / 60): void {
    // 1. Store input in history ring (evict oldest when full).
    const slot = this.historyHead;
    this.history[slot].seq = seq;
    this.history[slot].input = input;
    this.historyHead = (this.historyHead + 1) % MAX_HISTORY;
    if (this.historyCount < MAX_HISTORY) {
      this.historyCount += 1;
    }

    // 2. Advance predicted state by one step.
    const stateBuf = encodeState(this._predictedState);
    const inputBuf = encodeInput({
      seq,
      throttle: input.throttle,
      brake: input.brake,
      steer: input.steer,
      gear: input.gear,
      handbrake: input.handbrake,
    });
    const nextBuf = this.physics.step(stateBuf, inputBuf, dt);
    this._predictedState = decodeState(nextBuf);
  }

  // ---------------------------------------------------------------------------
  // applySnapshot — store authoritative server state
  // ---------------------------------------------------------------------------

  /**
   * Process an authoritative server snapshot. Stores the own-car state as
   * confirmedState/confirmedSeq. Does NOT mutate predictedState.
   *
   * If ownPlayerID is not in the snapshot's car list, this is a no-op.
   *
   * @param snap        Decoded ServerSnapshot.
   * @param ownPlayerID The local player's ID (from ServerHello).
   */
  applySnapshot(snap: ServerSnapshot, ownPlayerID: number): void {
    const car = snap.cars.find((c) => c.playerID === ownPlayerID);
    if (car === undefined) {
      // Own car not in snapshot — no-op.
      return;
    }

    // Convert protocol PhysicsState to wasm.PhysicsState (same shape).
    this._confirmedState = {
      position: car.state.position,
      orientation: car.state.orientation,
      linearVel: car.state.linearVel,
      angularVel: car.state.angularVel,
      rpm: car.state.rpm,
      gear: car.state.gear,
      wheelLoad: car.state.wheelLoad,
      wheelSlip: car.state.wheelSlip,
      grounded: car.state.grounded,
    };
    this._confirmedSeq = snap.lastAckedSeq;
    // predictedState is intentionally NOT updated here.
  }

  // ---------------------------------------------------------------------------
  // Diagnostics
  // ---------------------------------------------------------------------------

  /** Number of entries currently in the input history. */
  inputHistorySize(): number {
    return this.historyCount;
  }
}
