/**
 * web/src/protocol/messages.ts — TypeScript mirror of internal/protocol.
 *
 * Binary wire format: little-endian, 1-byte MsgType + 1-byte ProtocolVersion
 * followed by the message payload. See docs/protocol.md for byte-layout tables.
 *
 * Anti-cheat invariant: ClientInput MUST NOT contain any position, velocity,
 * lap time, or checkpoint field. Clients send inputs only; the server is the
 * sole authority on vehicle state.
 */

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

export const MsgType = {
  ClientInput: 0x01,
  ServerSnapshot: 0x02,
  ServerHello: 0x03,
} as const;

export type MsgTypeValue = (typeof MsgType)[keyof typeof MsgType];

export const PROTOCOL_VERSION = 1;

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

export class ProtocolError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ProtocolError';
  }
}

export class ShortBufferError extends ProtocolError {
  constructor() {
    super('protocol: buffer too short');
    this.name = 'ShortBufferError';
  }
}

export class UnknownMsgTypeError extends ProtocolError {
  constructor(got: number) {
    super(
      `protocol: unknown message type 0x${got.toString(16).padStart(2, '0')}`,
    );
    this.name = 'UnknownMsgTypeError';
  }
}

export class UnsupportedVersionError extends ProtocolError {
  constructor(got: number) {
    super(`protocol: unsupported protocol version ${got}`);
    this.name = 'UnsupportedVersionError';
  }
}

// ---------------------------------------------------------------------------
// Physics sub-types (mirrors internal/physics types.go)
// ---------------------------------------------------------------------------

/** Driver input frame. Anti-cheat: no position/velocity fields allowed here. */
export interface PhysicsInput {
  throttle: number; // float32 [0,1]
  brake: number; // float32 [0,1]
  steer: number; // float32 [-1,1]
  gear: number; // int8
  handbrake: boolean;
}

/** Vehicle state mirroring physics.State. */
export interface PhysicsState {
  position: [number, number, number]; // float32[3]
  orientation: [number, number, number, number]; // float32[4] xyzw
  linearVel: [number, number, number]; // float32[3]
  angularVel: [number, number, number]; // float32[3]
  rpm: number; // float32
  gear: number; // int8
  wheelLoad: [number, number, number, number]; // float32[4] FL,FR,RL,RR
  wheelSlip: [number, number, number, number]; // float32[4] FL,FR,RL,RR
  grounded: number; // uint8 bitmask
}

export interface TorqueSample {
  rpm: number; // float32
  torque: number; // float32
}

export interface PhysicsConstants {
  mass: number;
  wheelbase: number;
  trackWidth: number;
  maxEngineTorque: number;
  dragCoeff: number;
  downforceCoeff: number;
  tireGripCoeffs: [number, number, number, number]; // float32[4]
  maxSteerAngle: number;
  torqueCurve: [
    TorqueSample,
    TorqueSample,
    TorqueSample,
    TorqueSample,
    TorqueSample,
    TorqueSample,
    TorqueSample,
    TorqueSample,
  ]; // fixed 8 entries
  finalDrive: number;
  drivetrainEfficiency: number;
  wheelRadius: number;
  rollingResistCoeff: number;
  frontalArea: number;
  airDensity: number;
  maxBrakeForce: number;
  idleRPM: number;
  redlineRPM: number;
  pacejkaB: number;
  pacejkaC: number;
  pacejkaD: number;
  yawInertia: number;
  weightDistributionFront: number;
  cgHeight: number;
  rollStiffnessFront: number;
  gearRatios: number[]; // float32[], variable length
}

// ---------------------------------------------------------------------------
// Message types
// ---------------------------------------------------------------------------

/**
 * ClientInput — sent from client to server.
 * Anti-cheat: no position, velocity, or lap-time fields permitted here.
 */
export interface ClientInput {
  seq: number; // uint32
  clientTimeMs: number; // uint32
  input: PhysicsInput;
}

export interface CarState {
  playerID: number; // uint16
  nickname: string; // up to 8 bytes, null-padded on wire; over-long input truncated
  state: PhysicsState;
}

export interface ServerSnapshot {
  serverTickMs: number; // uint32
  lastAckedSeq: number; // uint32
  cars: CarState[];
}

export interface ServerHello {
  playerID: number; // uint16
  serverTickHz: number; // uint8
  snapshotHz: number; // uint8
  constants: PhysicsConstants;
}

// ---------------------------------------------------------------------------
// Size constants (bytes)
// ---------------------------------------------------------------------------

const HEADER_SIZE = 2;

/**
 * STATE_SIZE is the packed wire size of a physics.State (90 bytes, no padding).
 * Field order: Position[3], Orientation[4], LinearVel[3], AngularVel[3],
 *              RPM, Gear(int8), WheelLoad[4], WheelSlip[4], Grounded(uint8).
 * Matches docs/protocol.md and parity.test.ts:23-29.
 */
const STATE_SIZE = 90;

const CAR_STATE_SIZE = 2 + 8 + STATE_SIZE; // 100
const SNAPSHOT_HEADER_SIZE = HEADER_SIZE + 4 + 4 + 1; // 11
const CLIENT_INPUT_SIZE = HEADER_SIZE + 4 + 4 + 4 + 4 + 4 + 1 + 1; // 24
const CONSTANTS_FIXED_SIZE = 172;
// ServerHello minimum: header(2) + playerID(2) + 2 Hz bytes + 172 + gearLen(1)
const SERVER_HELLO_MIN_SIZE =
  HEADER_SIZE + 2 + 1 + 1 + CONSTANTS_FIXED_SIZE + 1; // 179

export function clientInputSize(): number {
  return CLIENT_INPUT_SIZE;
}

export function serverSnapshotSize(numCars: number): number {
  return SNAPSHOT_HEADER_SIZE + numCars * CAR_STATE_SIZE;
}

export function serverHelloSize(numGearRatios: number): number {
  return SERVER_HELLO_MIN_SIZE + numGearRatios * 4;
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

function checkHeader(view: DataView, expectedType: number): void {
  if (view.byteLength < 2) {
    throw new ShortBufferError();
  }
  const msgType = view.getUint8(0);
  if (msgType !== expectedType) {
    throw new UnknownMsgTypeError(msgType);
  }
  const version = view.getUint8(1);
  if (version !== PROTOCOL_VERSION) {
    throw new UnsupportedVersionError(version);
  }
}

/**
 * Write a physics.State into view starting at byte offset `off`.
 * Returns the new offset (off + STATE_SIZE).
 */
function writeState(view: DataView, off: number, s: PhysicsState): number {
  view.setFloat32(off, s.position[0], true); off += 4;
  view.setFloat32(off, s.position[1], true); off += 4;
  view.setFloat32(off, s.position[2], true); off += 4;
  view.setFloat32(off, s.orientation[0], true); off += 4;
  view.setFloat32(off, s.orientation[1], true); off += 4;
  view.setFloat32(off, s.orientation[2], true); off += 4;
  view.setFloat32(off, s.orientation[3], true); off += 4;
  view.setFloat32(off, s.linearVel[0], true); off += 4;
  view.setFloat32(off, s.linearVel[1], true); off += 4;
  view.setFloat32(off, s.linearVel[2], true); off += 4;
  view.setFloat32(off, s.angularVel[0], true); off += 4;
  view.setFloat32(off, s.angularVel[1], true); off += 4;
  view.setFloat32(off, s.angularVel[2], true); off += 4;
  view.setFloat32(off, s.rpm, true); off += 4;
  view.setInt8(off, s.gear); off += 1;
  view.setFloat32(off, s.wheelLoad[0], true); off += 4;
  view.setFloat32(off, s.wheelLoad[1], true); off += 4;
  view.setFloat32(off, s.wheelLoad[2], true); off += 4;
  view.setFloat32(off, s.wheelLoad[3], true); off += 4;
  view.setFloat32(off, s.wheelSlip[0], true); off += 4;
  view.setFloat32(off, s.wheelSlip[1], true); off += 4;
  view.setFloat32(off, s.wheelSlip[2], true); off += 4;
  view.setFloat32(off, s.wheelSlip[3], true); off += 4;
  view.setUint8(off, s.grounded); off += 1;
  return off;
}

/**
 * Read a physics.State from view starting at byte offset `off`.
 * Returns the decoded state and new offset (off + STATE_SIZE).
 */
function readState(view: DataView, off: number): [PhysicsState, number] {
  const px = view.getFloat32(off, true); off += 4;
  const py = view.getFloat32(off, true); off += 4;
  const pz = view.getFloat32(off, true); off += 4;
  const ox = view.getFloat32(off, true); off += 4;
  const oy = view.getFloat32(off, true); off += 4;
  const oz = view.getFloat32(off, true); off += 4;
  const ow = view.getFloat32(off, true); off += 4;
  const lx = view.getFloat32(off, true); off += 4;
  const ly = view.getFloat32(off, true); off += 4;
  const lz = view.getFloat32(off, true); off += 4;
  const ax = view.getFloat32(off, true); off += 4;
  const ay = view.getFloat32(off, true); off += 4;
  const az = view.getFloat32(off, true); off += 4;
  const rpm = view.getFloat32(off, true); off += 4;
  const gear = view.getInt8(off); off += 1;
  const wl0 = view.getFloat32(off, true); off += 4;
  const wl1 = view.getFloat32(off, true); off += 4;
  const wl2 = view.getFloat32(off, true); off += 4;
  const wl3 = view.getFloat32(off, true); off += 4;
  const ws0 = view.getFloat32(off, true); off += 4;
  const ws1 = view.getFloat32(off, true); off += 4;
  const ws2 = view.getFloat32(off, true); off += 4;
  const ws3 = view.getFloat32(off, true); off += 4;
  const grounded = view.getUint8(off); off += 1;
  return [
    {
      position: [px, py, pz],
      orientation: [ox, oy, oz, ow],
      linearVel: [lx, ly, lz],
      angularVel: [ax, ay, az],
      rpm,
      gear,
      wheelLoad: [wl0, wl1, wl2, wl3],
      wheelSlip: [ws0, ws1, ws2, ws3],
      grounded,
    },
    off,
  ];
}

/**
 * Write a nickname (up to 8 bytes, null-padded). Over-long input is silently
 * truncated.
 */
function writeNickname(view: DataView, off: number, nick: string): number {
  const enc = new TextEncoder().encode(nick);
  const len = Math.min(enc.length, 8);
  for (let i = 0; i < 8; i++) {
    view.setUint8(off + i, i < len ? enc[i] : 0);
  }
  return off + 8;
}

/**
 * Read an 8-byte null-padded nickname field.
 * Returns the decoded string and new offset (off + 8).
 */
function readNickname(view: DataView, off: number): [string, number] {
  const bytes: number[] = [];
  for (let i = 0; i < 8; i++) {
    const b = view.getUint8(off + i);
    if (b === 0) break;
    bytes.push(b);
  }
  return [new TextDecoder().decode(new Uint8Array(bytes)), off + 8];
}

// ---------------------------------------------------------------------------
// ClientInput encode/decode
// ---------------------------------------------------------------------------

/** Encode a ClientInput into a new Uint8Array (24 bytes). */
export function encodeClientInput(msg: ClientInput): Uint8Array {
  const buf = new Uint8Array(CLIENT_INPUT_SIZE);
  const view = new DataView(buf.buffer);
  view.setUint8(0, MsgType.ClientInput);
  view.setUint8(1, PROTOCOL_VERSION);
  view.setUint32(2, msg.seq, true);
  view.setUint32(6, msg.clientTimeMs, true);
  view.setFloat32(10, msg.input.throttle, true);
  view.setFloat32(14, msg.input.brake, true);
  view.setFloat32(18, msg.input.steer, true);
  view.setInt8(22, msg.input.gear);
  view.setUint8(23, msg.input.handbrake ? 1 : 0);
  return buf;
}

/** Decode a ClientInput from buf. */
export function decodeClientInput(buf: Uint8Array): ClientInput {
  const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
  checkHeader(view, MsgType.ClientInput);
  if (buf.byteLength < CLIENT_INPUT_SIZE) throw new ShortBufferError();
  return {
    seq: view.getUint32(2, true),
    clientTimeMs: view.getUint32(6, true),
    input: {
      throttle: view.getFloat32(10, true),
      brake: view.getFloat32(14, true),
      steer: view.getFloat32(18, true),
      gear: view.getInt8(22),
      handbrake: view.getUint8(23) !== 0,
    },
  };
}

// ---------------------------------------------------------------------------
// ServerSnapshot encode/decode
// ---------------------------------------------------------------------------

/** Encode a ServerSnapshot into a new Uint8Array. */
export function encodeServerSnapshot(msg: ServerSnapshot): Uint8Array {
  const buf = new Uint8Array(serverSnapshotSize(msg.cars.length));
  const view = new DataView(buf.buffer);
  view.setUint8(0, MsgType.ServerSnapshot);
  view.setUint8(1, PROTOCOL_VERSION);
  view.setUint32(2, msg.serverTickMs, true);
  view.setUint32(6, msg.lastAckedSeq, true);
  view.setUint8(10, msg.cars.length);
  let off = SNAPSHOT_HEADER_SIZE;
  for (const car of msg.cars) {
    view.setUint16(off, car.playerID, true);
    off += 2;
    off = writeNickname(view, off, car.nickname);
    off = writeState(view, off, car.state);
  }
  return buf;
}

/** Decode a ServerSnapshot from buf. */
export function decodeServerSnapshot(buf: Uint8Array): ServerSnapshot {
  const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
  checkHeader(view, MsgType.ServerSnapshot);
  if (buf.byteLength < SNAPSHOT_HEADER_SIZE) throw new ShortBufferError();
  const serverTickMs = view.getUint32(2, true);
  const lastAckedSeq = view.getUint32(6, true);
  const numCars = view.getUint8(10);
  if (buf.byteLength < SNAPSHOT_HEADER_SIZE + numCars * CAR_STATE_SIZE) {
    throw new ShortBufferError();
  }
  let off = SNAPSHOT_HEADER_SIZE;
  const cars: CarState[] = [];
  for (let i = 0; i < numCars; i++) {
    const playerID = view.getUint16(off, true);
    off += 2;
    const [nick, off2] = readNickname(view, off);
    off = off2;
    const [state, off3] = readState(view, off);
    off = off3;
    cars.push({ playerID, nickname: nick, state });
  }
  return { serverTickMs, lastAckedSeq, cars };
}

// ---------------------------------------------------------------------------
// ServerHello encode/decode
// ---------------------------------------------------------------------------

/** Encode a ServerHello into a new Uint8Array. */
export function encodeServerHello(msg: ServerHello): Uint8Array {
  const c = msg.constants;
  const buf = new Uint8Array(serverHelloSize(c.gearRatios.length));
  const view = new DataView(buf.buffer);
  view.setUint8(0, MsgType.ServerHello);
  view.setUint8(1, PROTOCOL_VERSION);
  view.setUint16(2, msg.playerID, true);
  view.setUint8(4, msg.serverTickHz);
  view.setUint8(5, msg.snapshotHz);
  let off = 6;
  view.setFloat32(off, c.mass, true); off += 4;
  view.setFloat32(off, c.wheelbase, true); off += 4;
  view.setFloat32(off, c.trackWidth, true); off += 4;
  view.setFloat32(off, c.maxEngineTorque, true); off += 4;
  view.setFloat32(off, c.dragCoeff, true); off += 4;
  view.setFloat32(off, c.downforceCoeff, true); off += 4;
  view.setFloat32(off, c.tireGripCoeffs[0], true); off += 4;
  view.setFloat32(off, c.tireGripCoeffs[1], true); off += 4;
  view.setFloat32(off, c.tireGripCoeffs[2], true); off += 4;
  view.setFloat32(off, c.tireGripCoeffs[3], true); off += 4;
  view.setFloat32(off, c.maxSteerAngle, true); off += 4;
  for (const ts of c.torqueCurve) {
    view.setFloat32(off, ts.rpm, true); off += 4;
    view.setFloat32(off, ts.torque, true); off += 4;
  }
  view.setFloat32(off, c.finalDrive, true); off += 4;
  view.setFloat32(off, c.drivetrainEfficiency, true); off += 4;
  view.setFloat32(off, c.wheelRadius, true); off += 4;
  view.setFloat32(off, c.rollingResistCoeff, true); off += 4;
  view.setFloat32(off, c.frontalArea, true); off += 4;
  view.setFloat32(off, c.airDensity, true); off += 4;
  view.setFloat32(off, c.maxBrakeForce, true); off += 4;
  view.setFloat32(off, c.idleRPM, true); off += 4;
  view.setFloat32(off, c.redlineRPM, true); off += 4;
  view.setFloat32(off, c.pacejkaB, true); off += 4;
  view.setFloat32(off, c.pacejkaC, true); off += 4;
  view.setFloat32(off, c.pacejkaD, true); off += 4;
  view.setFloat32(off, c.yawInertia, true); off += 4;
  view.setFloat32(off, c.weightDistributionFront, true); off += 4;
  view.setFloat32(off, c.cgHeight, true); off += 4;
  view.setFloat32(off, c.rollStiffnessFront, true); off += 4;
  view.setUint8(off, c.gearRatios.length); off += 1;
  for (const gr of c.gearRatios) {
    view.setFloat32(off, gr, true); off += 4;
  }
  return buf;
}

/** Decode a ServerHello from buf. */
export function decodeServerHello(buf: Uint8Array): ServerHello {
  const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
  checkHeader(view, MsgType.ServerHello);
  if (buf.byteLength < SERVER_HELLO_MIN_SIZE) throw new ShortBufferError();
  const playerID = view.getUint16(2, true);
  const serverTickHz = view.getUint8(4);
  const snapshotHz = view.getUint8(5);
  let off = 6;
  const mass = view.getFloat32(off, true); off += 4;
  const wheelbase = view.getFloat32(off, true); off += 4;
  const trackWidth = view.getFloat32(off, true); off += 4;
  const maxEngineTorque = view.getFloat32(off, true); off += 4;
  const dragCoeff = view.getFloat32(off, true); off += 4;
  const downforceCoeff = view.getFloat32(off, true); off += 4;
  const tgc0 = view.getFloat32(off, true); off += 4;
  const tgc1 = view.getFloat32(off, true); off += 4;
  const tgc2 = view.getFloat32(off, true); off += 4;
  const tgc3 = view.getFloat32(off, true); off += 4;
  const maxSteerAngle = view.getFloat32(off, true); off += 4;
  const torqueCurve: TorqueSample[] = [];
  for (let i = 0; i < 8; i++) {
    const rpm = view.getFloat32(off, true); off += 4;
    const torque = view.getFloat32(off, true); off += 4;
    torqueCurve.push({ rpm, torque });
  }
  const finalDrive = view.getFloat32(off, true); off += 4;
  const drivetrainEfficiency = view.getFloat32(off, true); off += 4;
  const wheelRadius = view.getFloat32(off, true); off += 4;
  const rollingResistCoeff = view.getFloat32(off, true); off += 4;
  const frontalArea = view.getFloat32(off, true); off += 4;
  const airDensity = view.getFloat32(off, true); off += 4;
  const maxBrakeForce = view.getFloat32(off, true); off += 4;
  const idleRPM = view.getFloat32(off, true); off += 4;
  const redlineRPM = view.getFloat32(off, true); off += 4;
  const pacejkaB = view.getFloat32(off, true); off += 4;
  const pacejkaC = view.getFloat32(off, true); off += 4;
  const pacejkaD = view.getFloat32(off, true); off += 4;
  const yawInertia = view.getFloat32(off, true); off += 4;
  const weightDistributionFront = view.getFloat32(off, true); off += 4;
  const cgHeight = view.getFloat32(off, true); off += 4;
  const rollStiffnessFront = view.getFloat32(off, true); off += 4;
  const numGear = view.getUint8(off); off += 1;
  if (buf.byteLength < off + numGear * 4) throw new ShortBufferError();
  const gearRatios: number[] = [];
  for (let i = 0; i < numGear; i++) {
    gearRatios.push(view.getFloat32(off, true));
    off += 4;
  }
  return {
    playerID,
    serverTickHz,
    snapshotHz,
    constants: {
      mass,
      wheelbase,
      trackWidth,
      maxEngineTorque,
      dragCoeff,
      downforceCoeff,
      tireGripCoeffs: [tgc0, tgc1, tgc2, tgc3],
      maxSteerAngle,
      torqueCurve: torqueCurve as PhysicsConstants['torqueCurve'],
      finalDrive,
      drivetrainEfficiency,
      wheelRadius,
      rollingResistCoeff,
      frontalArea,
      airDensity,
      maxBrakeForce,
      idleRPM,
      redlineRPM,
      pacejkaB,
      pacejkaC,
      pacejkaD,
      yawInertia,
      weightDistributionFront,
      cgHeight,
      rollStiffnessFront,
      gearRatios,
    },
  };
}
