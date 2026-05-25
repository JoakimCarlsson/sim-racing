/**
 * web/src/track/overlay.ts — Dev-overlay for the circuit01 track.
 *
 * TrackOverlay renders:
 *   1. A LineLoop polygon outlining the driveable surface (limitsPolygon).
 *   2. An ArrowHelper for each sector gate (normal direction).
 *   3. An ArrowHelper for the start/finish gate (normal direction).
 *
 * Usage:
 *   const overlay = new TrackOverlay(track, THREE);
 *   scene.add(overlay.root);          // add once
 *   overlay.setVisible(true/false);   // toggle with Backquote
 *   overlay.dispose();                // cleanup on unload
 *
 * The THREE parameter is the three.js namespace (or a compatible test stub)
 * so that this module is independently unit-testable without a DOM.
 */

import type { Track, Vec3 } from './load';

// ---------------------------------------------------------------------------
// Three.js type shim — the overlay accepts any object implementing this
// minimal subset of the three.js API.
// ---------------------------------------------------------------------------

export interface ThreeGroup {
  visible: boolean;
  children: object[];
  add(...objs: object[]): void;
  remove(...objs: object[]): void;
}

export interface ThreeGeometry {
  setAttribute(name: string, attr: object): void;
  dispose(): void;
}

export interface ThreeVector3 {
  x: number;
  y: number;
  z: number;
  set(x: number, y: number, z: number): ThreeVector3;
  normalize(): ThreeVector3;
  clone(): ThreeVector3;
}

/** Minimal three.js subset required by TrackOverlay. */
export interface ThreeAPI {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  Group: new (...args: any[]) => ThreeGroup;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  LineLoop: new (...args: any[]) => object;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  ArrowHelper: new (...args: any[]) => object;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  BufferGeometry: new (...args: any[]) => ThreeGeometry;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  Float32BufferAttribute: new (array: Float32Array, itemSize: number) => object;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  LineBasicMaterial: new (params: { color: number; depthTest: boolean }) => object;
  Vector3: new (x?: number, y?: number, z?: number) => ThreeVector3;
}

// ---------------------------------------------------------------------------
// TrackOverlay
// ---------------------------------------------------------------------------

export class TrackOverlay {
  /** The root group; add this to the scene. */
  public readonly root: ThreeGroup;

  /** Geometries to dispose on cleanup. */
  private readonly geometries: ThreeGeometry[] = [];

  constructor(track: Track, three: ThreeAPI) {
    this.root = new three.Group();
    this.root.visible = false; // hidden by default; toggled by Backquote

    // 1. Build the limits polygon LineLoop.
    this.addLimitsPolygon(track, three);

    // 2. Add an ArrowHelper for the start/finish gate.
    this.addPlaneArrow(track.startFinish, three, 0xffff00);

    // 3. Add an ArrowHelper for each sector gate.
    for (const sector of track.sectors) {
      this.addPlaneArrow(sector, three, 0x00ffff);
    }
  }

  /** Show or hide the entire overlay group. */
  setVisible(visible: boolean): void {
    this.root.visible = visible;
  }

  /** Release all geometries held by this overlay. */
  dispose(): void {
    // Remove all children from root.
    const children = [...this.root.children];
    this.root.remove(...children);

    // Dispose geometries.
    for (const geo of this.geometries) {
      geo.dispose();
    }
    this.geometries.length = 0;
  }

  // -------------------------------------------------------------------------
  // Private helpers
  // -------------------------------------------------------------------------

  private addLimitsPolygon(track: Track, three: ThreeAPI): void {
    const poly = track.limitsPolygon;
    // Build flat Float32Array of (x, 0, z) triples.
    const positions = new Float32Array(poly.length * 3);
    for (let i = 0; i < poly.length; i++) {
      positions[i * 3] = poly[i][0]; // x
      positions[i * 3 + 1] = 0.05; // slightly above ground to avoid z-fight
      positions[i * 3 + 2] = poly[i][1]; // z (track space)
    }

    const geo = new three.BufferGeometry();
    geo.setAttribute(
      'position',
      new three.Float32BufferAttribute(positions, 3),
    );
    this.geometries.push(geo);

    const mat = new three.LineBasicMaterial({
      color: 0xff4444,
      depthTest: false,
    });

    const loop = new three.LineLoop(geo, mat);
    this.root.add(loop);
  }

  private addPlaneArrow(
    plane: { p0: Vec3; p1: Vec3; normal: Vec3 },
    three: ThreeAPI,
    color: number,
  ): void {
    // Centroid of the gate (midpoint of P0 and P1).
    const cx = (plane.p0[0] + plane.p1[0]) * 0.5;
    const cy = (plane.p0[1] + plane.p1[1]) * 0.5 + 0.1;
    const cz = (plane.p0[2] + plane.p1[2]) * 0.5;

    const origin = new three.Vector3(cx, cy, cz);
    const dir = new three.Vector3(
      plane.normal[0],
      plane.normal[1],
      plane.normal[2],
    ).normalize();

    const arrow = new three.ArrowHelper(dir, origin, 3, color);
    this.root.add(arrow);
  }
}
