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
 */

import { encodeClientInput } from '../protocol/messages';
import type { InputSampler } from '../input/sampler';
import type { Socket } from './socket';

/** Minimum ms between two sent frames (125 Hz ceiling). */
export const MIN_INTERVAL_MS = 8;

/** Target interval in ms (60 Hz). */
export const TARGET_INTERVAL_MS = 1000 / 60; // ≈16.67

export class InputSender {
  /** Monotonic sequence number — persists across stop/start. */
  private seq = 0;
  private timer: ReturnType<typeof setInterval> | null = null;
  private lastSentAt = -Infinity;

  constructor(
    private readonly sampler: InputSampler,
    private readonly socket: Socket,
    private readonly now: () => number = () => performance.now(),
  ) {}

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
    const msg = encodeClientInput({
      seq: this.seq,
      clientTimeMs: Math.round(t) & 0xffffffff,
      input,
    });

    const sent = this.socket.sendBinary(msg);
    if (sent) {
      this.seq += 1;
      this.lastSentAt = t;
    }
  }
}
