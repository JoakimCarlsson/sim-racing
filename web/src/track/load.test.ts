/**
 * web/src/track/load.test.ts — Unit tests for the track JSON loader.
 *
 * Tests mirror the Go track_test.go validation cases, including:
 *   - Happy-path: valid document is accepted and typed correctly.
 *   - Sad-path: every validation rule has at least one rejection case.
 *   - All error messages are prefixed "[track]".
 *
 * loadTrackJSON is tested via a mock fetch to avoid network calls.
 * loadTrackGLTF is a thin delegator; it is smoke-tested with a mock loader.
 */

import { describe, it, expect, beforeAll, afterAll } from 'bun:test';
import { validateTrack, loadTrackJSON, loadTrackGLTF } from './load';
import type { Track } from './load';

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const GOOD_TRACK = {
  id: 'circuit01',
  name: 'Circuit 01',
  spawnPoints: [{ pos: [0, 0, 0], rot: [0, 0, 0, 1] }],
  startFinish: { p0: [-5, 0, 0], p1: [5, 0, 0], normal: [0, 0, 1] },
  sectors: [],
  limitsPolygon: [
    [-50, -50],
    [50, -50],
    [50, 50],
    [-50, 50],
  ],
};

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

function expectTrackError(input: unknown, expectedSubstring: string): void {
  expect(() => validateTrack(input)).toThrow(expectedSubstring);
}

// ---------------------------------------------------------------------------
// validateTrack — happy path
// ---------------------------------------------------------------------------

describe('validateTrack — happy path', () => {
  it('accepts a fully valid document', () => {
    const t: Track = validateTrack(GOOD_TRACK);
    expect(t.id).toBe('circuit01');
    expect(t.name).toBe('Circuit 01');
    expect(t.spawnPoints).toHaveLength(1);
    expect(t.spawnPoints[0].pos).toEqual([0, 0, 0]);
    expect(t.spawnPoints[0].rot).toEqual([0, 0, 0, 1]);
    expect(t.startFinish.normal).toEqual([0, 0, 1]);
    expect(t.sectors).toHaveLength(0);
    expect(t.limitsPolygon).toHaveLength(4);
  });

  it('accepts a document with one sector', () => {
    const withSector = {
      ...GOOD_TRACK,
      sectors: [{ p0: [-5, 0, 20], p1: [5, 0, 20], normal: [0, 0, 1] }],
    };
    const t: Track = validateTrack(withSector);
    expect(t.sectors).toHaveLength(1);
  });
});

// ---------------------------------------------------------------------------
// validateTrack — sad paths
// ---------------------------------------------------------------------------

describe('validateTrack — sad paths', () => {
  it('errors on non-object input', () => {
    expectTrackError(null, '[track]');
    expectTrackError(42, '[track]');
    expectTrackError('string', '[track]');
    expectTrackError([1, 2], '[track]');
  });

  it('errors on unknown field', () => {
    expectTrackError({ ...GOOD_TRACK, unknownField: true }, '[track] unknown field');
  });

  it('errors on missing id', () => {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    const { id: _id, ...noId } = GOOD_TRACK;
    expectTrackError(noId, '[track] id');
  });

  it('errors on empty id', () => {
    expectTrackError({ ...GOOD_TRACK, id: '' }, '[track] id');
  });

  it('errors on missing name', () => {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    const { name: _name, ...noName } = GOOD_TRACK;
    expectTrackError(noName, '[track] name');
  });

  it('errors on empty spawnPoints array', () => {
    expectTrackError({ ...GOOD_TRACK, spawnPoints: [] }, '[track] spawnPoints');
  });

  it('errors on non-array spawnPoints', () => {
    expectTrackError({ ...GOOD_TRACK, spawnPoints: 'bad' }, '[track] spawnPoints');
  });

  it('errors on limitsPolygon with fewer than 3 vertices', () => {
    expectTrackError(
      { ...GOOD_TRACK, limitsPolygon: [[-50, -50], [50, -50]] },
      '[track] limitsPolygon',
    );
  });

  it('errors on zero-vector startFinish normal', () => {
    expectTrackError(
      {
        ...GOOD_TRACK,
        startFinish: { p0: [-5, 0, 0], p1: [5, 0, 0], normal: [0, 0, 0] },
      },
      '[track]',
    );
  });

  it('errors on zero-vector sector normal', () => {
    expectTrackError(
      {
        ...GOOD_TRACK,
        sectors: [{ p0: [-5, 0, 10], p1: [5, 0, 10], normal: [0, 0, 0] }],
      },
      '[track]',
    );
  });

  it('errors on malformed spawnPoint.pos (wrong tuple length)', () => {
    expectTrackError(
      {
        ...GOOD_TRACK,
        spawnPoints: [{ pos: [0, 0], rot: [0, 0, 0, 1] }],
      },
      '[track]',
    );
  });

  it('errors on non-numeric limitsPolygon vertex', () => {
    expectTrackError(
      {
        ...GOOD_TRACK,
        limitsPolygon: [[-50, -50], [50, 'bad'], [50, 50]],
      },
      '[track]',
    );
  });
});

// ---------------------------------------------------------------------------
// loadTrackJSON — mock fetch
// ---------------------------------------------------------------------------

describe('loadTrackJSON', () => {
  let originalFetch: typeof globalThis.fetch;

  beforeAll(() => {
    originalFetch = globalThis.fetch;
  });

  afterAll(() => {
    globalThis.fetch = originalFetch;
  });

  function mockFetch(body: unknown, status = 200): void {
    // Cast through unknown to silence the TS strictness on the preconnect hint.
    globalThis.fetch = (async (
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      _url: string | URL | Request,
    ) => {
      return {
        ok: status >= 200 && status < 300,
        status,
        json: async () => body,
      } as Response;
    }) as typeof globalThis.fetch;
  }

  it('resolves with valid JSON', async () => {
    mockFetch(GOOD_TRACK);
    const t = await loadTrackJSON('/tracks/circuit01/track.json');
    expect(t.id).toBe('circuit01');
  });

  it('rejects with a [track] error on HTTP failure', async () => {
    mockFetch({}, 404);
    await expect(loadTrackJSON('/tracks/circuit01/track.json')).rejects.toThrow(
      '[track]',
    );
  });

  it('rejects with a [track] error on invalid JSON body', async () => {
    mockFetch({ id: '', name: 'Bad' }); // empty id → validation failure
    await expect(loadTrackJSON('/tracks/circuit01/track.json')).rejects.toThrow(
      '[track]',
    );
  });
});

// ---------------------------------------------------------------------------
// loadTrackGLTF — mock loader
// ---------------------------------------------------------------------------

describe('loadTrackGLTF', () => {
  it('calls loader.loadAsync and returns scene.Group', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const fakeScene = { type: 'Group' } as any;
    const mockLoader = {
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      loadAsync: async (_url: string) => ({ scene: fakeScene }),
    };
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const result = await loadTrackGLTF('/tracks/circuit01/track.gltf', mockLoader as any);
    expect(result).toBe(fakeScene);
  });
});
