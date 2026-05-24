/**
 * web/src/input/sampler.test.ts — InputSampler unit tests with fake clock.
 *
 * AC3: steer ramp 0→±1 in 150 ms, ±1→0 in 100 ms.
 */

import { describe, test, expect } from 'bun:test';
import { InputSampler, STEER_RAMP_UP_MS, STEER_RAMP_DOWN_MS } from './sampler';
import type { KeyboardSource } from './keyboard';
import type { KeySnapshot } from './keyboard';

// ---------------------------------------------------------------------------
// Fake KeyboardSource
// ---------------------------------------------------------------------------

interface FakeKeys {
  throttle?: boolean;
  brake?: boolean;
  steerLeft?: boolean;
  steerRight?: boolean;
  handbrake?: boolean;
  gearDelta?: number;
}

function makeSource(initial: FakeKeys = {}): KeyboardSource {
  let state: FakeKeys = { ...initial };
  let pendingGearDelta = initial.gearDelta ?? 0;

  const src = {
    snapshot(): KeySnapshot {
      return {
        throttle: state.throttle ?? false,
        brake: state.brake ?? false,
        steerLeft: state.steerLeft ?? false,
        steerRight: state.steerRight ?? false,
        handbrake: state.handbrake ?? false,
      };
    },
    consumeGearDelta(): number {
      const d = Math.sign(pendingGearDelta);
      pendingGearDelta = 0;
      return d;
    },
    dispose(): void {},
    // Test helpers
    set(keys: FakeKeys): void {
      state = { ...state, ...keys };
      if (keys.gearDelta !== undefined) pendingGearDelta += keys.gearDelta;
    },
  };
  return src as unknown as KeyboardSource;
}

// ---------------------------------------------------------------------------
// (a) Throttle / brake / handbrake are digital (0 or 1)
// ---------------------------------------------------------------------------

describe('InputSampler digital controls', () => {
  test('(a) throttle=1 when W held, brake=1 when S held, handbrake=true when Space held', () => {
    let t = 0;
    const src = makeSource({ throttle: true, brake: false, handbrake: true });
    const sampler = new InputSampler(src, () => (t += 16));

    const input = sampler.tick();
    expect(input.throttle).toBe(1);
    expect(input.brake).toBe(0);
    expect(input.handbrake).toBe(true);
    // throttle/brake in [0,1]
    expect(input.throttle).toBeGreaterThanOrEqual(0);
    expect(input.throttle).toBeLessThanOrEqual(1);
    expect(input.brake).toBeGreaterThanOrEqual(0);
    expect(input.brake).toBeLessThanOrEqual(1);
  });

  test('(a) steer stays in [-1,1] range at all times', () => {
    let t = 0;
    const src = makeSource({ steerRight: true });
    const sampler = new InputSampler(src, () => (t += 50));

    // Advance well past ramp time
    for (let i = 0; i < 10; i++) {
      const input = sampler.tick();
      expect(input.steer).toBeGreaterThanOrEqual(-1);
      expect(input.steer).toBeLessThanOrEqual(1);
    }
  });
});

// ---------------------------------------------------------------------------
// (b) Steer ramp up: 150ms to reach ±1
// ---------------------------------------------------------------------------

describe('InputSampler steer ramp up', () => {
  test('(b) steer reaches -1 after exactly STEER_RAMP_UP_MS when steerLeft held', () => {
    // Use a single tick of exactly ramp time
    let t = 0;
    const src = makeSource({ steerLeft: true });
    const sampler = new InputSampler(src, () => t);

    // First tick: dt = 0 (same time) — no movement
    t = 0;
    sampler.tick(); // sets lastNow = 0

    t = STEER_RAMP_UP_MS;
    const input = sampler.tick();
    expect(input.steer).toBeCloseTo(-1, 5);
  });

  test('(b) steer reaches +1 after exactly STEER_RAMP_UP_MS when steerRight held', () => {
    let t = 0;
    const src = makeSource({ steerRight: true });
    const sampler = new InputSampler(src, () => t);

    t = 0;
    sampler.tick();

    t = STEER_RAMP_UP_MS;
    const input = sampler.tick();
    expect(input.steer).toBeCloseTo(1, 5);
  });

  test('(b) steer is ~0.5 halfway through ramp-up', () => {
    let t = 0;
    const src = makeSource({ steerRight: true });
    const sampler = new InputSampler(src, () => t);

    t = 0;
    sampler.tick();

    t = STEER_RAMP_UP_MS / 2;
    const input = sampler.tick();
    // Allow small float tolerance
    expect(input.steer).toBeCloseTo(0.5, 4);
  });
});

// ---------------------------------------------------------------------------
// (c) Steer ramp down: 100ms back to 0
// ---------------------------------------------------------------------------

describe('InputSampler steer ramp down', () => {
  test('(c) steer returns to 0 after exactly STEER_RAMP_DOWN_MS when key released', () => {
    let t = 0;
    const src = makeSource({ steerRight: true });
    const sampler = new InputSampler(src, () => t);

    // Ramp up to +1
    t = 0;
    sampler.tick();
    t = STEER_RAMP_UP_MS;
    sampler.tick(); // steer = 1

    // Release key
    (src as unknown as { set(k: FakeKeys): void }).set({ steerRight: false });

    t = STEER_RAMP_UP_MS + STEER_RAMP_DOWN_MS;
    const input = sampler.tick();
    expect(input.steer).toBeCloseTo(0, 5);
  });

  test('(c) steer halfway at half ramp-down time', () => {
    let t = 0;
    const src = makeSource({ steerLeft: true });
    const sampler = new InputSampler(src, () => t);

    t = 0;
    sampler.tick();
    t = STEER_RAMP_UP_MS;
    sampler.tick(); // steer = -1

    (src as unknown as { set(k: FakeKeys): void }).set({ steerLeft: false });

    t = STEER_RAMP_UP_MS + STEER_RAMP_DOWN_MS / 2;
    const input = sampler.tick();
    expect(input.steer).toBeCloseTo(-0.5, 4);
  });
});

// ---------------------------------------------------------------------------
// (d) Gear delta via consumeGearDelta
// ---------------------------------------------------------------------------

describe('InputSampler gear', () => {
  test('(d) gear increments on E, decrements on Q', () => {
    let t = 0;
    const src = makeSource();
    const sampler = new InputSampler(src, () => (t += 16));

    // Initial tick: gear = 1
    const first = sampler.tick();
    expect(first.gear).toBe(1);

    // Gear up
    (src as unknown as { set(k: FakeKeys): void }).set({ gearDelta: 1 });
    const up = sampler.tick();
    expect(up.gear).toBe(2);

    // Gear down twice — but consumeGearDelta saturates to -1 per tick
    (src as unknown as { set(k: FakeKeys): void }).set({ gearDelta: -1 });
    const down = sampler.tick();
    expect(down.gear).toBe(1);
  });

  test('(d) gear clamps at -1 (reverse)', () => {
    let t = 0;
    const src = makeSource();
    const sampler = new InputSampler(src, () => (t += 16));

    // Start at gear 1; spam gear-down
    for (let i = 0; i < 5; i++) {
      (src as unknown as { set(k: FakeKeys): void }).set({ gearDelta: -1 });
      sampler.tick();
    }
    const input = sampler.tick();
    expect(input.gear).toBeGreaterThanOrEqual(-1);
  });

  test('(d) gear clamps at 8', () => {
    let t = 0;
    const src = makeSource();
    const sampler = new InputSampler(src, () => (t += 16));

    for (let i = 0; i < 15; i++) {
      (src as unknown as { set(k: FakeKeys): void }).set({ gearDelta: 1 });
      sampler.tick();
    }
    const input = sampler.tick();
    expect(input.gear).toBeLessThanOrEqual(8);
  });
});
