/**
 * web/src/input/keyboard.ts — Keyboard input source.
 *
 * Tracks key state for the standard driving controls:
 *   W / S   → throttle / brake
 *   A / D   → steer left / right
 *   Q / E   → gear down / up
 *   Space   → handbrake
 *
 * Usage:
 *   const kb = new KeyboardSource();
 *   const snap = kb.snapshot();   // raw key booleans
 *   const delta = kb.consumeGearDelta(); // -1 | 0 | +1
 *   kb.dispose();
 */

export interface KeySnapshot {
  throttle: boolean;
  brake: boolean;
  steerLeft: boolean;
  steerRight: boolean;
  handbrake: boolean;
}

export class KeyboardSource {
  private readonly keys = new Set<string>();
  private gearDelta = 0;

  private readonly onDown: (e: KeyboardEvent) => void;
  private readonly onUp: (e: KeyboardEvent) => void;

  constructor(target: EventTarget = window) {
    this.onDown = (e: KeyboardEvent) => {
      if (e.repeat) return;
      this.keys.add(e.code);
      if (e.code === 'KeyE') this.gearDelta += 1;
      if (e.code === 'KeyQ') this.gearDelta -= 1;
    };
    this.onUp = (e: KeyboardEvent) => {
      this.keys.delete(e.code);
    };
    target.addEventListener('keydown', this.onDown as EventListener);
    target.addEventListener('keyup', this.onUp as EventListener);
  }

  /** Current snapshot of raw key states (no gear delta). */
  snapshot(): KeySnapshot {
    return {
      throttle: this.keys.has('KeyW'),
      brake: this.keys.has('KeyS'),
      steerLeft: this.keys.has('KeyA'),
      steerRight: this.keys.has('KeyD'),
      handbrake: this.keys.has('Space'),
    };
  }

  /**
   * Returns accumulated gear change since last call (+1 up, -1 down, 0 none),
   * then resets the accumulator. Multiple presses between ticks are collapsed
   * to ±1 (saturate to avoid large jumps).
   */
  consumeGearDelta(): number {
    const delta = Math.sign(this.gearDelta);
    this.gearDelta = 0;
    return delta;
  }

  /** Remove event listeners. */
  dispose(): void {
    window.removeEventListener('keydown', this.onDown as EventListener);
    window.removeEventListener('keyup', this.onUp as EventListener);
  }
}
