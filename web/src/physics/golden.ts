/**
 * web/src/physics/golden.ts — paths to test fixtures used by parity.test.ts.
 *
 * These paths resolve relative to the repository root so that bun test can
 * read them via Node's fs API regardless of the working directory.
 */
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

/** Absolute path to the input fixture binary (20 bytes/record, LE). */
export const LAP_INPUTS_BIN = path.resolve(
  __dirname,
  '../../../internal/physics/testdata/lap_inputs.bin',
);

/** Absolute path to the committed golden SHA-256 hex digest. */
export const LAP_INPUTS_GOLDEN = path.resolve(
  __dirname,
  '../../../internal/physics/testdata/lap_inputs.golden',
);

/** Absolute path to the compiled physics WASM binary. */
export const PHYSICS_WASM = path.resolve(
  __dirname,
  '../../public/physics.wasm',
);

/** Absolute path to the Go WASM runtime shim. */
export const WASM_EXEC_JS = path.resolve(
  __dirname,
  '../../public/wasm_exec.js',
);
