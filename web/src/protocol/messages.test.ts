/**
 * web/src/protocol/messages.test.ts
 *
 * Golden round-trip: read each golden_*.bin produced by the Go test suite,
 * assert field values match the known fixture, re-encode, assert byte-identical
 * output. This proves Go and TypeScript agree on the wire format.
 *
 * Reads fixtures from internal/protocol/testdata/ (same as parity.test.ts
 * pattern using fileURLToPath + path.resolve).
 */

import { describe, test, expect } from 'bun:test';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

import {
  decodeClientInput,
  encodeClientInput,
  decodeServerSnapshot,
  encodeServerSnapshot,
  decodeServerHello,
  encodeServerHello,
  MsgType,
  PROTOCOL_VERSION,
  ShortBufferError,
  UnknownMsgTypeError,
  UnsupportedVersionError,
} from './messages';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const TESTDATA = path.resolve(
  __dirname,
  '../../../internal/protocol/testdata',
);

function loadGolden(name: string): Uint8Array {
  const data = fs.readFileSync(path.join(TESTDATA, name));
  return new Uint8Array(data.buffer, data.byteOffset, data.byteLength);
}

function assertBytesEqual(label: string, got: Uint8Array, want: Uint8Array): void {
  expect(got.byteLength).toBe(want.byteLength);
  for (let i = 0; i < want.byteLength; i++) {
    if (got[i] !== want[i]) {
      throw new Error(
        `${label}: byte[${i}] want 0x${want[i].toString(16).padStart(2, '0')}, got 0x${got[i].toString(16).padStart(2, '0')}`,
      );
    }
  }
}

// ---------------------------------------------------------------------------
// Golden round-trip tests (AC3)
// ---------------------------------------------------------------------------

describe('golden round-trip', () => {
  test('ClientInput decode+re-encode is byte-identical to golden', () => {
    const golden = loadGolden('golden_client_input.bin');

    // Header
    expect(golden[0]).toBe(MsgType.ClientInput);
    expect(golden[1]).toBe(PROTOCOL_VERSION);

    // Decode
    const msg = decodeClientInput(golden);
    expect(msg.seq).toBe(1);
    expect(msg.clientTimeMs).toBe(1000);
    expect(msg.input.throttle).toBeCloseTo(0.75, 5);
    expect(msg.input.brake).toBeCloseTo(0.0, 5);
    expect(msg.input.steer).toBeCloseTo(-0.5, 5);
    expect(msg.input.gear).toBe(2);
    expect(msg.input.handbrake).toBe(false);

    // Re-encode must be byte-identical
    const reEncoded = encodeClientInput(msg);
    assertBytesEqual('ClientInput', reEncoded, golden);
  });

  test('ServerSnapshot decode+re-encode is byte-identical to golden', () => {
    const golden = loadGolden('golden_server_snapshot.bin');

    expect(golden[0]).toBe(MsgType.ServerSnapshot);
    expect(golden[1]).toBe(PROTOCOL_VERSION);

    const msg = decodeServerSnapshot(golden);
    expect(msg.serverTickMs).toBe(5000);
    expect(msg.lastAckedSeq).toBe(42);
    expect(msg.cars).toHaveLength(1);

    const car = msg.cars[0];
    expect(car.playerID).toBe(7);
    expect(car.nickname).toBe('player01');

    const s = car.state;
    expect(s.position[0]).toBeCloseTo(1.0, 5);
    expect(s.position[1]).toBeCloseTo(2.0, 5);
    expect(s.position[2]).toBeCloseTo(3.0, 5);
    expect(s.orientation[0]).toBeCloseTo(0.0, 5);
    expect(s.orientation[3]).toBeCloseTo(1.0, 5);
    expect(s.linearVel[0]).toBeCloseTo(10.0, 5);
    expect(s.linearVel[2]).toBeCloseTo(20.0, 5);
    expect(s.rpm).toBeCloseTo(3500.0, 5);
    expect(s.gear).toBe(3);
    expect(s.wheelLoad[0]).toBeCloseTo(2500.0, 3);
    expect(s.wheelLoad[2]).toBeCloseTo(2700.0, 3);
    expect(s.wheelSlip[0]).toBeCloseTo(0.05, 5);
    expect(s.grounded).toBe(0x0f);

    const reEncoded = encodeServerSnapshot(msg);
    assertBytesEqual('ServerSnapshot', reEncoded, golden);
  });

  test('ServerHello decode+re-encode is byte-identical to golden', () => {
    const golden = loadGolden('golden_server_hello.bin');

    expect(golden[0]).toBe(MsgType.ServerHello);
    expect(golden[1]).toBe(PROTOCOL_VERSION);

    const msg = decodeServerHello(golden);
    expect(msg.playerID).toBe(7);
    expect(msg.serverTickHz).toBe(60);
    expect(msg.snapshotHz).toBe(20);

    const c = msg.constants;
    expect(c.mass).toBeCloseTo(1200, 0);
    expect(c.redlineRPM).toBeCloseTo(7000, 0);
    expect(c.gearRatios).toHaveLength(8);
    expect(c.gearRatios[0]).toBeCloseTo(-2.9, 3);
    expect(c.gearRatios[2]).toBeCloseTo(2.5, 3);
    expect(c.torqueCurve).toHaveLength(8);
    expect(c.torqueCurve[0].rpm).toBeCloseTo(900, 0);
    expect(c.torqueCurve[3].torque).toBeCloseTo(310, 0);

    const reEncoded = encodeServerHello(msg);
    assertBytesEqual('ServerHello', reEncoded, golden);
  });
});

// ---------------------------------------------------------------------------
// Sentinel error tests
// ---------------------------------------------------------------------------

describe('sentinel errors', () => {
  test('ShortBufferError on empty buffer', () => {
    expect(() => decodeClientInput(new Uint8Array(0))).toThrow(
      ShortBufferError,
    );
  });

  test('UnknownMsgTypeError on wrong type byte', () => {
    const buf = new Uint8Array(24);
    buf[0] = 0xff;
    buf[1] = PROTOCOL_VERSION;
    expect(() => decodeClientInput(buf)).toThrow(UnknownMsgTypeError);
  });

  test('UnsupportedVersionError on wrong version byte', () => {
    const buf = new Uint8Array(24);
    buf[0] = MsgType.ClientInput;
    buf[1] = 0xff;
    expect(() => decodeClientInput(buf)).toThrow(UnsupportedVersionError);
  });

  test('ShortBufferError on truncated ClientInput', () => {
    const buf = new Uint8Array(4);
    buf[0] = MsgType.ClientInput;
    buf[1] = PROTOCOL_VERSION;
    expect(() => decodeClientInput(buf)).toThrow(ShortBufferError);
  });
});
