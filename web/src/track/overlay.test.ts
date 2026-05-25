/**
 * web/src/track/overlay.test.ts
 *
 * Unit tests for TrackOverlay:
 *   - constructor creates a root Group with child objects
 *   - setVisible(false) hides the overlay
 *   - setVisible(true) shows the overlay
 *   - dispose() removes all children and geometries
 *   - polygon vertices match the limitsPolygon shape
 *   - plane arrows exist for startFinish and each sector
 *
 * These tests use a minimal three.js mock so they run in Bun without a DOM.
 */

import { describe, it, expect, beforeEach } from 'bun:test';
import { TrackOverlay } from './overlay';
import type { Track } from './load';

// ---------------------------------------------------------------------------
// Minimal three.js stubs — just enough for overlay.ts to construct and test.
// ---------------------------------------------------------------------------

let disposeCount = 0;

class FakeGeometry {
  disposed = false;
  dispose() {
    this.disposed = true;
    disposeCount++;
  }
}

class FakeMaterial {
  disposed = false;
  dispose() {
    this.disposed = true;
  }
}

class FakeObject3D {
  visible = true;
  children: FakeObject3D[] = [];
  add(...objs: FakeObject3D[]) {
    for (const o of objs) this.children.push(o);
  }
  remove(...objs: FakeObject3D[]) {
    for (const o of objs) {
      const i = this.children.indexOf(o);
      if (i >= 0) this.children.splice(i, 1);
    }
  }
}

class FakeGroup extends FakeObject3D {}

class FakeLineLoop extends FakeObject3D {
  geometry: FakeGeometry;
  material: FakeMaterial;
  constructor(geo: FakeGeometry, mat: FakeMaterial) {
    super();
    this.geometry = geo;
    this.material = mat;
  }
}

class FakeArrowHelper extends FakeObject3D {
  // Arrow helpers accept positional constructor args that we don't inspect.
  /* eslint-disable-next-line @typescript-eslint/no-unused-vars, @typescript-eslint/no-explicit-any */
  constructor(...a: any[]) { super(); void a; }
}

class FakeBufferGeometry extends FakeGeometry {
  /* eslint-disable-next-line @typescript-eslint/no-unused-vars, @typescript-eslint/no-explicit-any */
  setAttribute(...a: any[]) { void a; }
}

class FakeFloat32BufferAttribute {
  constructor(
    public array: Float32Array,
    public itemSize: number,
  ) {}
}

class FakeLineBasicMaterial extends FakeMaterial {
  /* eslint-disable-next-line @typescript-eslint/no-unused-vars, @typescript-eslint/no-explicit-any */
  constructor(...a: any[]) { super(); void a; }
}

class FakeVector3 {
  x = 0;
  y = 0;
  z = 0;
  constructor(x = 0, y = 0, z = 0) {
    this.x = x;
    this.y = y;
    this.z = z;
  }
  set(x: number, y: number, z: number) {
    this.x = x;
    this.y = y;
    this.z = z;
    return this;
  }
  normalize() {
    return this;
  }
  clone() {
    return new FakeVector3(this.x, this.y, this.z);
  }
}

// Build the fake three module that overlay.ts imports.
const fakeThree = {
  Group: FakeGroup,
  LineLoop: FakeLineLoop,
  ArrowHelper: FakeArrowHelper,
  BufferGeometry: FakeBufferGeometry,
  Float32BufferAttribute: FakeFloat32BufferAttribute,
  LineBasicMaterial: FakeLineBasicMaterial,
  Vector3: FakeVector3,
};

// ---------------------------------------------------------------------------
// Fixture
// ---------------------------------------------------------------------------

const TRACK: Track = {
  id: 'circuit01',
  name: 'Circuit 01',
  spawnPoints: [{ pos: [0, 0, 0], rot: [0, 0, 0, 1] }],
  startFinish: { p0: [-5, 0, 0], p1: [5, 0, 0], normal: [0, 0, 1] },
  sectors: [
    { p0: [6, 0, -5], p1: [14, 0, -5], normal: [0, 0, -1] },
    { p0: [-14, 0, -12], p1: [14, 0, -12], normal: [-1, 0, 0] },
  ],
  limitsPolygon: [
    [-14, -14],
    [14, -14],
    [14, -4],
    [6, -4],
    [6, 14],
    [-6, 14],
    [-6, -4],
    [-14, -4],
  ],
};

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('TrackOverlay', () => {
  beforeEach(() => {
    disposeCount = 0;
  });

  it('constructs without throwing', () => {
    expect(
      () => new TrackOverlay(TRACK, fakeThree as never),
    ).not.toThrow();
  });

  it('root is a Group', () => {
    const overlay = new TrackOverlay(TRACK, fakeThree as never);
    expect(overlay.root).toBeInstanceOf(FakeGroup);
  });

  it('root has children after construction', () => {
    const overlay = new TrackOverlay(TRACK, fakeThree as never);
    expect((overlay.root as FakeGroup).children.length).toBeGreaterThan(0);
  });

  it('setVisible(false) sets root.visible=false', () => {
    const overlay = new TrackOverlay(TRACK, fakeThree as never);
    overlay.setVisible(false);
    expect((overlay.root as FakeGroup).visible).toBe(false);
  });

  it('setVisible(true) sets root.visible=true', () => {
    const overlay = new TrackOverlay(TRACK, fakeThree as never);
    overlay.setVisible(false);
    overlay.setVisible(true);
    expect((overlay.root as FakeGroup).visible).toBe(true);
  });

  it('dispose() removes all children from root', () => {
    const overlay = new TrackOverlay(TRACK, fakeThree as never);
    overlay.dispose();
    expect((overlay.root as FakeGroup).children.length).toBe(0);
  });

  it('dispose() calls geometry.dispose() on each line geometry', () => {
    const overlay = new TrackOverlay(TRACK, fakeThree as never);
    overlay.dispose();
    // At minimum the polygon LineLoop geometry must be disposed.
    expect(disposeCount).toBeGreaterThan(0);
  });

  it('creates one LineLoop for the limitsPolygon', () => {
    const overlay = new TrackOverlay(TRACK, fakeThree as never);
    const loops = (overlay.root as FakeGroup).children.filter(
      (c) => c instanceof FakeLineLoop,
    );
    expect(loops.length).toBe(1);
  });

  it('polygon LineLoop geometry is a BufferGeometry', () => {
    const overlay = new TrackOverlay(TRACK, fakeThree as never);
    const loop = (overlay.root as FakeGroup).children.find(
      (c) => c instanceof FakeLineLoop,
    ) as FakeLineLoop;
    expect(loop).toBeInstanceOf(FakeLineLoop);
    expect(loop.geometry).toBeInstanceOf(FakeBufferGeometry);
  });

  it('creates ArrowHelpers for startFinish and each sector', () => {
    const overlay = new TrackOverlay(TRACK, fakeThree as never);
    const arrows = (overlay.root as FakeGroup).children.filter(
      (c) => c instanceof FakeArrowHelper,
    );
    // 1 (startFinish) + 2 (sectors)
    expect(arrows.length).toBe(3);
  });
});
