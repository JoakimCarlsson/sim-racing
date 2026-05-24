/**
 * web/src/sim/predictor.ts — Client-side prediction with input-replay reconciliation.
 *
 * The Predictor maintains a rolling window of at most MAX_HISTORY=256 unacked
 * input frames. On each tick() it advances the predicted state one step forward
 * using the compiled WASM physics (bit-for-bit identical to the server).
 *
 * applySnapshot() now performs full reconciliation:
 *   1. Store the confirmed state and acked seq.
 *   2. Walk the history ring oldest-first; drop entries with seq <= confirmedSeq.
 *   3. Re-simulate remaining entries from confirmedState to produce a new
 *      predictedState that is consistent with the server's authoritative view.
 *   4. Compact the ring to keep only unacked entries.
 *   5. Compute predictionError = difference between old and new predicted states.
 *   6. Apply visual smoothing: small errors ease over ~100ms; large errors snap.
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

/** Position smoothing threshold in metres. Errors below this ease; above snap. */
const POS_SMOOTH_M = 0.5;

/** Yaw smoothing threshold in radians (5 degrees). */
const YAW_SMOOTH_RAD = 5 * (Math.PI / 180);

/** Smoothing time constant in milliseconds. */
const SMOOTH_TAU_MS = 100;

/** An entry in the input history ring buffer. */
interface HistoryEntry {
  seq: number;
  input: PhysicsInput;
}

/** Prediction error computed after reconciliation. */
export interface PredictionError {
  /** Euclidean distance in metres between old and new predicted positions. */
  positionDelta: number;
  /** Absolute yaw difference in radians between old and new predicted orientations. */
  yawDeltaRad: number;
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

/**
 * Extract yaw (rotation around Y) from a quaternion [qx, qy, qz, qw].
 * Uses the standard formula: atan2(2*(qw*qy + qx*qz), 1 - 2*(qy^2 + qz^2))
 *
 * NOTE: Three.js / our wire format uses orientation = [qx, qy, qz, qw].
 */
function yawFromQuat(o: [number, number, number, number]): number {
  const qx = o[0], qy = o[1], qz = o[2], qw = o[3];
  return Math.atan2(2 * (qw * qy + qx * qz), 1 - 2 * (qy * qy + qz * qz));
}

/** Euclidean distance between two 3-element positions. */
function posDelta(a: [number, number, number], b: [number, number, number]): number {
  const dx = a[0] - b[0];
  const dy = a[1] - b[1];
  const dz = a[2] - b[2];
  return Math.sqrt(dx * dx + dy * dy + dz * dz);
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
   * The last computed prediction error (old vs new predicted state after
   * reconciliation). Null before the first snapshot is applied.
   */
  private _predictionError: PredictionError | null = null;

  /**
   * Visual smoothing offset: renderState = predictedState + smoothingOffset.
   * When a small reconciliation occurs, offset starts at (oldPredicted - newPredicted)
   * so the visible position stays at the old location and eases toward zero.
   * Stored as a 3-element position offset; yaw is not smoothed independently.
   */
  private _smoothingOffset: [number, number, number] = [0, 0, 0];

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

  /**
   * Prediction error computed during the most recent applySnapshot reconciliation.
   * Null before the first snapshot is applied.
   */
  get predictionError(): PredictionError | null {
    return this._predictionError;
  }

  /**
   * Render state: predictedState offset by any active visual smoothing.
   * Use this to drive the visual representation of the car instead of
   * predictedState directly.
   */
  get renderState(): Readonly<PhysicsState> {
    const off = this._smoothingOffset;
    // Fast-path: no offset active.
    if (off[0] === 0 && off[1] === 0 && off[2] === 0) {
      return this._predictedState;
    }
    const p = this._predictedState;
    return {
      position: [
        p.position[0] + off[0],
        p.position[1] + off[1],
        p.position[2] + off[2],
      ],
      orientation: p.orientation,
      linearVel: p.linearVel,
      angularVel: p.angularVel,
      rpm: p.rpm,
      gear: p.gear,
      wheelLoad: p.wheelLoad,
      wheelSlip: p.wheelSlip,
      grounded: p.grounded,
    };
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

    // 3. Decay visual smoothing offset exponentially (τ = SMOOTH_TAU_MS ms).
    const off = this._smoothingOffset;
    if (off[0] !== 0 || off[1] !== 0 || off[2] !== 0) {
      const decay = Math.exp(-dt * 1000 / SMOOTH_TAU_MS);
      this._smoothingOffset = [
        off[0] * decay,
        off[1] * decay,
        off[2] * decay,
      ];
    }
  }

  // ---------------------------------------------------------------------------
  // applySnapshot — store authoritative server state + reconcile
  // ---------------------------------------------------------------------------

  /**
   * Process an authoritative server snapshot. Stores the own-car state as
   * confirmedState/confirmedSeq, then replays unacked inputs from that state
   * to produce an updated predictedState. Computes predictionError and sets
   * the visual smoothing offset.
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

    // Store confirmed state.
    const confirmedState: PhysicsState = {
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
    this._confirmedState = confirmedState;
    this._confirmedSeq = snap.lastAckedSeq;

    const confirmedSeq = snap.lastAckedSeq;

    // Capture old predicted state for error computation.
    const oldPredicted = this._predictedState;

    // -----------------------------------------------------------------------
    // Collect unacked inputs: history entries with seq > confirmedSeq,
    // ordered oldest-first.
    // -----------------------------------------------------------------------
    const unacked: HistoryEntry[] = [];

    if (this.historyCount > 0) {
      const oldestIdx = (this.historyHead - this.historyCount + MAX_HISTORY) % MAX_HISTORY;

      // Check edge case: lastAckedSeq < oldest history seq.
      // This means we have no record of inputs before confirmedSeq — snap.
      const oldestSeq = this.history[oldestIdx].seq;
      if (confirmedSeq < oldestSeq - 1) {
        // Server acked a seq older than our oldest history entry.
        // We can't reliably replay, so warn and snap to confirmedState.
        console.warn(
          `[predictor] lastAckedSeq=${confirmedSeq} < oldest history seq=${oldestSeq}; ` +
          'snapping to confirmedState with empty replay.',
        );
        this._predictedState = confirmedState;
        this._predictionError = {
          positionDelta: posDelta(oldPredicted.position, confirmedState.position),
          yawDeltaRad: Math.abs(
            yawFromQuat(oldPredicted.orientation) - yawFromQuat(confirmedState.orientation),
          ),
        };
        // Snap — no smoothing offset.
        this._smoothingOffset = [0, 0, 0];
        this.historyCount = 0;
        this.historyHead = 0;
        return;
      }

      for (let i = 0; i < this.historyCount; i++) {
        const idx = (oldestIdx + i) % MAX_HISTORY;
        const entry = this.history[idx];
        if (entry.seq > confirmedSeq) {
          unacked.push({ seq: entry.seq, input: { ...entry.input } });
        }
      }
    }

    // -----------------------------------------------------------------------
    // Replay unacked inputs from confirmedState.
    // -----------------------------------------------------------------------
    let replayBuf = encodeState(confirmedState);
    for (const entry of unacked) {
      const inputBuf = encodeInput({
        seq: entry.seq,
        throttle: entry.input.throttle,
        brake: entry.input.brake,
        steer: entry.input.steer,
        gear: entry.input.gear,
        handbrake: entry.input.handbrake,
      });
      replayBuf = this.physics.step(replayBuf, inputBuf, 1 / 60);
    }
    const newPredicted = decodeState(replayBuf);

    // -----------------------------------------------------------------------
    // Compute prediction error.
    // -----------------------------------------------------------------------
    const posDeltaVal = posDelta(oldPredicted.position, newPredicted.position);
    const yawDeltaVal = Math.abs(
      yawFromQuat(oldPredicted.orientation) - yawFromQuat(newPredicted.orientation),
    );
    this._predictionError = {
      positionDelta: posDeltaVal,
      yawDeltaRad: yawDeltaVal,
    };

    // -----------------------------------------------------------------------
    // Visual smoothing.
    // -----------------------------------------------------------------------
    if (posDeltaVal < POS_SMOOTH_M && yawDeltaVal < YAW_SMOOTH_RAD) {
      // Small error — ease from old position to new over ~100ms.
      this._smoothingOffset = [
        oldPredicted.position[0] - newPredicted.position[0],
        oldPredicted.position[1] - newPredicted.position[1],
        oldPredicted.position[2] - newPredicted.position[2],
      ];
    } else {
      // Large error — snap immediately.
      this._smoothingOffset = [0, 0, 0];
    }

    // -----------------------------------------------------------------------
    // Update predicted state and compact ring to unacked entries only.
    // -----------------------------------------------------------------------
    this._predictedState = newPredicted;

    // Compact: rewrite ring with only unacked entries.
    this.historyCount = unacked.length;
    this.historyHead = unacked.length % MAX_HISTORY;
    for (let i = 0; i < unacked.length; i++) {
      this.history[i].seq = unacked[i].seq;
      this.history[i].input = { ...unacked[i].input };
    }
  }

  // ---------------------------------------------------------------------------
  // Diagnostics
  // ---------------------------------------------------------------------------

  /** Number of entries currently in the input history. */
  inputHistorySize(): number {
    return this.historyCount;
  }
}
