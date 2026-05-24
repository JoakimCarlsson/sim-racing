/**
 * web/src/net/inputSender.ts — 60 Hz input sender loop.
 *
 * - Calls sampler.tick() at ~60 Hz (setInterval 1000/60 ≈ 16.67 ms).
 * - Encodes the PhysicsInput as a ClientInput binary frame (24 bytes).
 * - Sends via socket.sendBinary(); drops silently if socket not open.
 * - seq is monotonically increasing and NEVER resets on disconnect/reconnect.
 * - start() / stop() are idempotent.
 *
 * Min-interval guard: frames sent faster than MIN_INTERVAL_MS are skipped
 * (protects against setInterval firing twice in one JS tick).
 *
 * opts.onTick is called AFTER a successful send with (seq, input) where seq
 * is the sequence number that was just sent. This ensures the predictor never
 * sees a re-used seq (failed sends are silently dropped — no onTick call).
 */

import { encodeClientInput } from '../protocol/messages';
import type { PhysicsInput } from '../protocol/messages';
import type { InputSampler } from '../input/sampler';
import type { Socket } from './socket';

/** Minimum ms between two sent frames (125 Hz ceiling). */
export const MIN_INTERVAL_MS = 8;

/** Target interval in ms (60 Hz). */
export const TARGET_INTERVAL_MS = 1000 / 60; // ≈16.67

/** Callback fired after each successful send. */
export type OnTickCallback = (seq: number, input: PhysicsInput) => void;

export interface InputSenderOpts {
  /** Custom clock; defaults to performance.now(). */
  now?: () => number;
  /**
   * Called immediately after a successful sendBinary(), with the seq number
   * that was just sent and the raw input. Failed sends (socket closed, rate
   * guard) do NOT trigger this callback.
   *
   * Default: no-op.
   */
  onTick?: OnTickCallback;
}

export class InputSender {
  /** Monotonic sequence number — persists across stop/start. */
  private seq = 0;
  private timer: ReturnType<typeof setInterval> | null = null;
  private lastSentAt = -Infinity;
  private readonly now: () => number;
  private readonly onTick: OnTickCallback;

  constructor(
    private readonly sampler: InputSampler,
    private readonly socket: Socket,
    optsOrNow: InputSenderOpts | (() => number) = {},
  ) {
    // Backward-compatible: old callers pass `now` as a bare function.
    if (typeof optsOrNow === 'function') {
      this.now = optsOrNow;
      this.onTick = () => {};
    } else {
      this.now = optsOrNow.now ?? (() => performance.now());
      this.onTick = optsOrNow.onTick ?? (() => {});
    }
  }

  /** Start the 60 Hz send loop. Idempotent. */
  start(): void {
    if (this.timer !== null) return;
    this.timer = setInterval(() => this.flush(), TARGET_INTERVAL_MS);
  }

  /** Stop the send loop. Idempotent. */
  stop(): void {
    if (this.timer === null) return;
    clearInterval(this.timer);
    this.timer = null;
  }

  private flush(): void {
    const t = this.now();
    if (t - this.lastSentAt < MIN_INTERVAL_MS) return;

    if (!this.socket.isOpen()) return;

    const input = this.sampler.tick();
    const sentSeq = this.seq;
    const msg = encodeClientInput({
      seq: sentSeq,
      clientTimeMs: Math.round(t) & 0xffffffff,
      input,
    });

    const sent = this.socket.sendBinary(msg);
    if (sent) {
      this.seq += 1;
      this.lastSentAt = t;
      // Fire onTick AFTER seq has already been incremented, passing the seq
      // that was sent (sentSeq, not this.seq which is already the next).
      this.onTick(sentSeq, input);
    }
  }
}
