/**
 * web/src/input/sampler.ts — Input sampler with steer ramping.
 *
 * Converts raw key booleans into a normalised PhysicsInput.
 *
 * Steer ramp:
 *   - Key held: ramps from 0 → ±1 over STEER_RAMP_UP_MS (150 ms).
 *   - Key released: ramps back to 0 over STEER_RAMP_DOWN_MS (100 ms).
 *
 * Throttle / brake are digital (0 or 1) — analogue input is a non-goal.
 *
 * The `now` function is injected for deterministic testing.
 */

import type { PhysicsInput } from '../protocol/messages';
import type { KeyboardSource } from './keyboard';

/** Time (ms) to ramp steer from 0 → ±1 when key held. */
export const STEER_RAMP_UP_MS = 150;

/** Time (ms) to ramp steer from ±1 → 0 when key released. */
export const STEER_RAMP_DOWN_MS = 100;

export class InputSampler {
  private steer = 0;
  private gear = 1;
  private lastNow: number;

  constructor(
    private readonly source: KeyboardSource,
    private readonly now: () => number = () => performance.now(),
  ) {
    this.lastNow = now();
  }

  /**
   * Sample the current key state and advance the steer ramp by dt.
   * Returns a fully normalised PhysicsInput.
   */
  tick(): PhysicsInput {
    const currentNow = this.now();
    const dt = currentNow - this.lastNow;
    this.lastNow = currentNow;

    const snap = this.source.snapshot();

    // Steer ramping
    const steerLeft = snap.steerLeft;
    const steerRight = snap.steerRight;

    if (steerLeft && !steerRight) {
      // Ramp toward -1
      const step = dt / STEER_RAMP_UP_MS;
      this.steer = Math.max(-1, this.steer - step);
    } else if (steerRight && !steerLeft) {
      // Ramp toward +1
      const step = dt / STEER_RAMP_UP_MS;
      this.steer = Math.min(1, this.steer + step);
    } else {
      // Neither or both → decay toward 0
      const step = dt / STEER_RAMP_DOWN_MS;
      if (this.steer > 0) {
        this.steer = Math.max(0, this.steer - step);
      } else if (this.steer < 0) {
        this.steer = Math.min(0, this.steer + step);
      }
    }

    // Gear change
    const delta = this.source.consumeGearDelta();
    this.gear += delta;
    // Clamp to sensible range (reverse=-1, neutral=0, gears 1-8)
    this.gear = Math.max(-1, Math.min(8, this.gear));

    return {
      throttle: snap.throttle ? 1 : 0,
      brake: snap.brake ? 1 : 0,
      steer: this.steer,
      gear: this.gear,
      handbrake: snap.handbrake,
    };
  }

  /** Reset steer and gear state (e.g. on disconnect). */
  reset(): void {
    this.steer = 0;
    this.gear = 1;
    this.lastNow = this.now();
  }
}
