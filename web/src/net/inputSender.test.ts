/**
 * web/src/net/inputSender.test.ts — InputSender unit tests.
 *
 * AC2: ~60 frames/sec when socket open.
 * AC4: seq monotonic, never repeats across disconnect/reconnect.
 * onTick: called N times with monotonic seq on successful send; NOT called on
 *         failed send (socket closed or rate-guard skip).
 */

import { describe, test, expect } from 'bun:test';
import { InputSender, MIN_INTERVAL_MS, TARGET_INTERVAL_MS } from './inputSender';
import type { InputSampler } from '../input/sampler';
import type { Socket } from './socket';
import type { PhysicsInput } from '../protocol/messages';

// ---------------------------------------------------------------------------
// Fake helpers
// ---------------------------------------------------------------------------

function makeFakeSampler(): InputSampler {
  const sampler = {
    tick(): PhysicsInput {
      return { throttle: 1, brake: 0, steer: 0, gear: 1, handbrake: false };
    },
    reset(): void {},
  };
  return sampler as unknown as InputSampler;
}

interface FakeSocket {
  socket: Socket;
  frames: Uint8Array[];
  openState: boolean;
}

function makeFakeSocket(): FakeSocket {
  const frames: Uint8Array[] = [];
  let openState = true;

  const socket = {
    isOpen(): boolean {
      return openState;
    },
    sendBinary(data: Uint8Array): boolean {
      if (!openState) return false;
      frames.push(new Uint8Array(data));
      return true;
    },
    connect(): void {},
    send(): void {},
    onMessage(): void {},
    onBinaryMessage(): void {},
    onOpen(): void {},
    onClose(): void {},
  };

  return {
    socket: socket as unknown as Socket,
    frames,
    get openState() {
      return openState;
    },
    set openState(v: boolean) {
      openState = v;
    },
  };
}

// ---------------------------------------------------------------------------
// AC4: seq monotonic, never repeats across reconnects
// ---------------------------------------------------------------------------

describe('InputSender seq', () => {
  test('(AC4) start/stop is idempotent — no throw', () => {
    const fake = makeFakeSocket();
    const sender = new InputSender(makeFakeSampler(), fake.socket, () => performance.now());

    sender.start();
    sender.start(); // idempotent
    sender.stop();
    sender.stop(); // idempotent

    expect(true).toBe(true);
  });

  test('(AC4) seq is monotonic across simulated reconnects', async () => {
    const fake = makeFakeSocket();
    const sender = new InputSender(makeFakeSampler(), fake.socket, () => performance.now());

    sender.start();
    await new Promise((resolve) => setTimeout(resolve, 120)); // ~7 ticks at 60Hz

    // "Disconnect"
    fake.openState = false;
    sender.stop();

    const seqAtDisconnect = fake.frames.length;
    expect(seqAtDisconnect).toBeGreaterThan(0);

    // Validate seq values in frames are monotonically increasing from 0
    for (let i = 0; i < fake.frames.length; i++) {
      const frame = fake.frames[i];
      const view = new DataView(frame.buffer, frame.byteOffset, frame.byteLength);
      const seq = view.getUint32(2, true); // bytes 2-5 little-endian
      expect(seq).toBe(i);
    }

    // "Reconnect" — seq must NOT reset
    fake.openState = true;
    sender.start();
    await new Promise((resolve) => setTimeout(resolve, 120)); // ~7 more ticks
    sender.stop();

    expect(fake.frames.length).toBeGreaterThan(seqAtDisconnect);

    // Frames after reconnect must continue from where seq left off
    for (let i = seqAtDisconnect; i < fake.frames.length; i++) {
      const frame = fake.frames[i];
      const view = new DataView(frame.buffer, frame.byteOffset, frame.byteLength);
      const seq = view.getUint32(2, true);
      expect(seq).toBe(i);
    }
  });

  test('(AC4) no frame sent when socket closed', async () => {
    const fake = makeFakeSocket();
    fake.openState = false;
    const sender = new InputSender(makeFakeSampler(), fake.socket, () => performance.now());

    sender.start();
    await new Promise((resolve) => setTimeout(resolve, 120));
    sender.stop();

    expect(fake.frames).toHaveLength(0);
  });

  test('(AC2) sends approximately 60 frames/sec over 500ms (15-35 frames expected)', async () => {
    const fake = makeFakeSocket();
    // Use real performance.now() so the sender sees actual elapsed time
    const sender = new InputSender(makeFakeSampler(), fake.socket, () => performance.now());

    sender.start();
    await new Promise((resolve) => setTimeout(resolve, 500));
    sender.stop();

    // At 60 Hz over 500ms we expect ~30 frames.
    // Bun test-runner timers may fire slower; allow 15-35 range.
    // The important property: interval is ~16.67ms, not coarser than 33ms.
    const frames = fake.frames.length;
    expect(frames).toBeGreaterThanOrEqual(15);
    expect(frames).toBeLessThanOrEqual(35);
  });

  test('(AC2/AC4) MIN_INTERVAL_MS=8 (125Hz ceiling) and TARGET_INTERVAL_MS≈16.67ms', () => {
    expect(MIN_INTERVAL_MS).toBe(8);
    expect(TARGET_INTERVAL_MS).toBeCloseTo(1000 / 60, 1);
  });
});

// ---------------------------------------------------------------------------
// onTick callback tests
// ---------------------------------------------------------------------------

describe('InputSender onTick', () => {
  test('onTick called with monotonic seq on each successful send', async () => {
    const fake = makeFakeSocket();
    const seqsReceived: number[] = [];

    const sender = new InputSender(makeFakeSampler(), fake.socket, {
      now: () => performance.now(),
      onTick: (seq) => {
        seqsReceived.push(seq);
      },
    });

    sender.start();
    await new Promise((resolve) => setTimeout(resolve, 200)); // ~12 ticks
    sender.stop();

    // Must have received at least some ticks.
    expect(seqsReceived.length).toBeGreaterThan(0);

    // Seq values must be monotonically increasing from 0.
    for (let i = 0; i < seqsReceived.length; i++) {
      expect(seqsReceived[i]).toBe(i);
    }

    // Number of frames sent == number of onTick calls.
    expect(seqsReceived.length).toBe(fake.frames.length);
  });

  test('onTick NOT called when socket is closed (failed send)', async () => {
    const fake = makeFakeSocket();
    fake.openState = false; // socket starts closed

    let tickCount = 0;
    const sender = new InputSender(makeFakeSampler(), fake.socket, {
      now: () => performance.now(),
      onTick: () => {
        tickCount += 1;
      },
    });

    sender.start();
    await new Promise((resolve) => setTimeout(resolve, 200));
    sender.stop();

    expect(tickCount).toBe(0);
    expect(fake.frames).toHaveLength(0);
  });

  test('backward-compatible: passing now as bare function still works', async () => {
    const fake = makeFakeSocket();
    // Old-style constructor: third arg is a bare () => number function.
    const sender = new InputSender(makeFakeSampler(), fake.socket, () => performance.now());

    sender.start();
    await new Promise((resolve) => setTimeout(resolve, 120));
    sender.stop();

    // Should have sent some frames without throwing.
    expect(fake.frames.length).toBeGreaterThan(0);
  });
});
