/**
 * web/src/track/load.ts — Track JSON loader and validator (TypeScript).
 *
 * Mirrors the canonical schema from docs/track-format.md and the validation
 * rules from internal/track/track.go.  All errors are prefixed "[track]".
 *
 * Exports:
 *   - Track types (Vec2, Vec3, Quat, Plane, SpawnPoint, Track)
 *   - loadTrackJSON(url) — fetch + validate
 *   - validateTrack(unknown) — type guard / validator; throws on bad input
 *   - loadTrackGLTF(url, loader) — delegate to the provided three.js loader
 */

import type { Group } from 'three';

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

/** 3-component float vector [x, y, z]. */
export type Vec3 = [number, number, number];

/** 2-component float vector [x, z] for the 2-D limits polygon. */
export type Vec2 = [number, number];

/** Unit quaternion [x, y, z, w]. */
export type Quat = [number, number, number, number];

/**
 * A gate plane defined by two endpoints and an outward normal.
 * p0/p1 are the left and right endpoints; normal faces track-forward.
 */
export interface Plane {
  p0: Vec3;
  p1: Vec3;
  normal: Vec3;
}

/** A spawn position and orientation. */
export interface SpawnPoint {
  pos: Vec3;
  rot: Quat;
}

/** Canonical in-memory track representation. */
export interface Track {
  id: string;
  name: string;
  spawnPoints: SpawnPoint[];
  startFinish: Plane;
  sectors: Plane[];
  /** Counter-clockwise (x,z) outline of the driveable surface. */
  limitsPolygon: Vec2[];
}

// ---------------------------------------------------------------------------
// Loader
// ---------------------------------------------------------------------------

/**
 * Fetch and validate a track.json document.
 * Throws with a "[track]" prefix on any validation failure.
 */
export async function loadTrackJSON(url: string): Promise<Track> {
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`[track] fetch ${url}: HTTP ${res.status}`);
  }
  const raw: unknown = await res.json();
  return validateTrack(raw);
}

/**
 * Validate an unknown value as a Track.  Throws a "[track]" error on any
 * violation, mirroring the rules in internal/track/track.go Validate().
 */
export function validateTrack(t: unknown): Track {
  if (typeof t !== 'object' || t === null || Array.isArray(t)) {
    throw new Error('[track] document must be a JSON object');
  }

  const obj = t as Record<string, unknown>;

  // Reject unexpected top-level keys (mirrors DisallowUnknownFields).
  const allowed = new Set([
    'id',
    'name',
    'spawnPoints',
    'startFinish',
    'sectors',
    'limitsPolygon',
  ]);
  for (const key of Object.keys(obj)) {
    if (!allowed.has(key)) {
      throw new Error(`[track] unknown field: "${key}"`);
    }
  }

  if (typeof obj['id'] !== 'string' || obj['id'] === '') {
    throw new Error('[track] id must be a non-empty string');
  }
  if (typeof obj['name'] !== 'string' || obj['name'] === '') {
    throw new Error('[track] name must be a non-empty string');
  }

  const spawnPoints = requireArray(obj['spawnPoints'], 'spawnPoints');
  if (spawnPoints.length === 0) {
    throw new Error('[track] spawnPoints must contain at least one entry');
  }
  const parsedSpawns: SpawnPoint[] = spawnPoints.map((s, i) =>
    parseSpawnPoint(s, `spawnPoints[${i}]`),
  );

  const startFinish = parsePlane(obj['startFinish'], 'startFinish');

  const sectorsRaw = requireArray(obj['sectors'], 'sectors');
  const sectors: Plane[] = sectorsRaw.map((s, i) =>
    parsePlane(s, `sectors[${i}]`),
  );

  const polyRaw = requireArray(obj['limitsPolygon'], 'limitsPolygon');
  if (polyRaw.length < 3) {
    throw new Error(
      `[track] limitsPolygon must have at least 3 vertices, got ${polyRaw.length}`,
    );
  }
  const limitsPolygon: Vec2[] = polyRaw.map((v, i) =>
    parseVec2(v, `limitsPolygon[${i}]`),
  );

  return {
    id: obj['id'] as string,
    name: obj['name'] as string,
    spawnPoints: parsedSpawns,
    startFinish,
    sectors,
    limitsPolygon,
  };
}

/**
 * Load a track glTF model.  The caller must supply a three.js GLTFLoader
 * instance (or compatible loader) so this module does not hard-depend on
 * the three.js package at import time.
 *
 * @param url    URL to the track.gltf file.
 * @param loader A three.js GLTFLoader (or duck-typed equivalent).
 * @returns      The root Group from the loaded GLTF scene.
 */
export async function loadTrackGLTF(
  url: string,
  loader: { loadAsync(url: string): Promise<{ scene: Group }> },
): Promise<Group> {
  const gltf = await loader.loadAsync(url);
  return gltf.scene;
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

function requireArray(v: unknown, field: string): unknown[] {
  if (!Array.isArray(v)) {
    throw new Error(`[track] ${field} must be an array`);
  }
  return v;
}

function parseVec3(v: unknown, field: string): Vec3 {
  if (!Array.isArray(v) || v.length !== 3 || v.some((x) => typeof x !== 'number')) {
    throw new Error(`[track] ${field} must be [number, number, number]`);
  }
  return [v[0] as number, v[1] as number, v[2] as number];
}

function parseVec2(v: unknown, field: string): Vec2 {
  if (!Array.isArray(v) || v.length !== 2 || v.some((x) => typeof x !== 'number')) {
    throw new Error(`[track] ${field} must be [number, number]`);
  }
  return [v[0] as number, v[1] as number];
}

function parseQuat(v: unknown, field: string): Quat {
  if (!Array.isArray(v) || v.length !== 4 || v.some((x) => typeof x !== 'number')) {
    throw new Error(`[track] ${field} must be [number, number, number, number]`);
  }
  return [v[0] as number, v[1] as number, v[2] as number, v[3] as number];
}

function assertNonZeroNormal(n: Vec3, field: string): void {
  if (n[0] === 0 && n[1] === 0 && n[2] === 0) {
    throw new Error(`[track] ${field} must not be a zero vector`);
  }
}

function parsePlane(v: unknown, field: string): Plane {
  if (typeof v !== 'object' || v === null || Array.isArray(v)) {
    throw new Error(`[track] ${field} must be an object`);
  }
  const obj = v as Record<string, unknown>;
  const p0 = parseVec3(obj['p0'], `${field}.p0`);
  const p1 = parseVec3(obj['p1'], `${field}.p1`);
  const normal = parseVec3(obj['normal'], `${field}.normal`);
  assertNonZeroNormal(normal, `${field}.normal`);
  return { p0, p1, normal };
}

function parseSpawnPoint(v: unknown, field: string): SpawnPoint {
  if (typeof v !== 'object' || v === null || Array.isArray(v)) {
    throw new Error(`[track] ${field} must be an object`);
  }
  const obj = v as Record<string, unknown>;
  const pos = parseVec3(obj['pos'], `${field}.pos`);
  const rot = parseQuat(obj['rot'], `${field}.rot`);
  return { pos, rot };
}
