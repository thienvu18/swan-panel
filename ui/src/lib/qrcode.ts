/**
 * Self-contained ISO/IEC 18004 Byte-mode QR Code matrix generator (Versions 1-12, ECC Level M).
 * Enables offline/air-gapped authenticator provisioning with zero third-party network calls.
 */

interface VersionSpec {
  version: number;
  ecPerBlock: number;
  blocksGroup1: number;
  dataPerBlock1: number;
  blocksGroup2: number;
  dataPerBlock2: number;
}

// ISO/IEC 18004 Table 9 — Error Correction Level M (Versions 1 through 12)
const VERSION_SPECS_M: VersionSpec[] = [
  { version: 1, ecPerBlock: 10, blocksGroup1: 1, dataPerBlock1: 16, blocksGroup2: 0, dataPerBlock2: 0 },
  { version: 2, ecPerBlock: 16, blocksGroup1: 1, dataPerBlock1: 28, blocksGroup2: 0, dataPerBlock2: 0 },
  { version: 3, ecPerBlock: 26, blocksGroup1: 1, dataPerBlock1: 44, blocksGroup2: 0, dataPerBlock2: 0 },
  { version: 4, ecPerBlock: 18, blocksGroup1: 2, dataPerBlock1: 32, blocksGroup2: 0, dataPerBlock2: 0 },
  { version: 5, ecPerBlock: 24, blocksGroup1: 2, dataPerBlock1: 43, blocksGroup2: 0, dataPerBlock2: 0 },
  { version: 6, ecPerBlock: 16, blocksGroup1: 4, dataPerBlock1: 27, blocksGroup2: 0, dataPerBlock2: 0 },
  { version: 7, ecPerBlock: 18, blocksGroup1: 4, dataPerBlock1: 31, blocksGroup2: 0, dataPerBlock2: 0 },
  { version: 8, ecPerBlock: 22, blocksGroup1: 2, dataPerBlock1: 38, blocksGroup2: 2, dataPerBlock2: 39 },
  { version: 9, ecPerBlock: 22, blocksGroup1: 3, dataPerBlock1: 36, blocksGroup2: 2, dataPerBlock2: 37 },
  { version: 10, ecPerBlock: 26, blocksGroup1: 4, dataPerBlock1: 43, blocksGroup2: 1, dataPerBlock2: 44 },
  { version: 11, ecPerBlock: 30, blocksGroup1: 1, dataPerBlock1: 50, blocksGroup2: 4, dataPerBlock2: 51 },
  { version: 12, ecPerBlock: 22, blocksGroup1: 6, dataPerBlock1: 36, blocksGroup2: 2, dataPerBlock2: 37 },
];

const ALIGNMENT_COORDS: Record<number, number[]> = {
  1: [],
  2: [6, 18],
  3: [6, 22],
  4: [6, 26],
  5: [6, 30],
  6: [6, 34],
  7: [6, 22, 38],
  8: [6, 24, 42],
  9: [6, 26, 46],
  10: [6, 28, 50],
  11: [6, 30, 54],
  12: [6, 32, 58],
};

// GF(256) log/antilog tables with primitive polynomial x^8 + x^4 + x^3 + x^2 + 1 (0x11d)
const GF_EXP = new Uint8Array(512);
const GF_LOG = new Uint8Array(256);
(() => {
  let x = 1;
  for (let i = 0; i < 255; i++) {
    GF_EXP[i] = x;
    GF_LOG[x] = i;
    x <<= 1;
    if (x & 0x100) x ^= 0x11d;
  }
  for (let i = 255; i < 512; i++) {
    GF_EXP[i] = GF_EXP[i - 255];
  }
})();

function gfMul(a: number, b: number): number {
  if (a === 0 || b === 0) return 0;
  return GF_EXP[GF_LOG[a] + GF_LOG[b]];
}

function rsGeneratorPoly(degree: number): Uint8Array {
  let poly = new Uint8Array([1]);
  for (let i = 0; i < degree; i++) {
    const next = new Uint8Array(poly.length + 1);
    const factor = GF_EXP[i];
    for (let j = 0; j < poly.length; j++) {
      next[j] ^= poly[j];
      next[j + 1] ^= gfMul(poly[j], factor);
    }
    poly = next;
  }
  return poly;
}

function rsComputeEC(data: Uint8Array, ecCount: number): Uint8Array {
  const gen = rsGeneratorPoly(ecCount);
  const work = new Uint8Array(data.length + ecCount);
  work.set(data, 0);
  for (let i = 0; i < data.length; i++) {
    const coef = work[i];
    if (coef !== 0) {
      for (let j = 1; j < gen.length; j++) {
        work[i + j] ^= gfMul(gen[j], coef);
      }
    }
  }
  return work.slice(data.length);
}

function appendBits(buffer: number[], value: number, bitCount: number) {
  for (let i = bitCount - 1; i >= 0; i--) {
    buffer.push((value >>> i) & 1);
  }
}

function selectSpecAndEncode(bytes: Uint8Array): { spec: VersionSpec; codewords: Uint8Array } {
  for (const spec of VERSION_SPECS_M) {
    const totalDataCodewords =
      spec.blocksGroup1 * spec.dataPerBlock1 + spec.blocksGroup2 * spec.dataPerBlock2;
    const countBits = spec.version <= 9 ? 8 : 16;
    const neededBits = 4 + countBits + bytes.length * 8;
    if (neededBits <= totalDataCodewords * 8) {
      const bits: number[] = [];
      appendBits(bits, 0b0100, 4); // Byte mode
      appendBits(bits, bytes.length, countBits);
      for (const b of bytes) appendBits(bits, b, 8);

      const capBits = totalDataCodewords * 8;
      const terminator = Math.min(4, capBits - bits.length);
      for (let i = 0; i < terminator; i++) bits.push(0);
      while (bits.length % 8 !== 0) bits.push(0);

      const dataBytes = new Uint8Array(totalDataCodewords);
      for (let i = 0; i < bits.length; i += 8) {
        let val = 0;
        for (let b = 0; b < 8; b++) val = (val << 1) | bits[i + b];
        dataBytes[i / 8] = val;
      }

      const padPatterns = [0xec, 0x11];
      let padIdx = 0;
      for (let i = bits.length / 8; i < totalDataCodewords; i++) {
        dataBytes[i] = padPatterns[padIdx++ % 2];
      }

      // Split into EC blocks and interleave
      const dataBlocks: Uint8Array[] = [];
      const ecBlocks: Uint8Array[] = [];
      let offset = 0;
      for (let b = 0; b < spec.blocksGroup1; b++) {
        const slice = dataBytes.slice(offset, offset + spec.dataPerBlock1);
        offset += spec.dataPerBlock1;
        dataBlocks.push(slice);
        ecBlocks.push(rsComputeEC(slice, spec.ecPerBlock));
      }
      for (let b = 0; b < spec.blocksGroup2; b++) {
        const slice = dataBytes.slice(offset, offset + spec.dataPerBlock2);
        offset += spec.dataPerBlock2;
        dataBlocks.push(slice);
        ecBlocks.push(rsComputeEC(slice, spec.ecPerBlock));
      }

      const maxDataLen = Math.max(spec.dataPerBlock1, spec.dataPerBlock2);
      const result: number[] = [];
      for (let i = 0; i < maxDataLen; i++) {
        for (const blk of dataBlocks) {
          if (i < blk.length) result.push(blk[i]);
        }
      }
      for (let i = 0; i < spec.ecPerBlock; i++) {
        for (const blk of ecBlocks) result.push(blk[i]);
      }

      return { spec, codewords: new Uint8Array(result) };
    }
  }
  throw new Error('Input exceeds maximum QR capacity (Version 12-M)');
}

function drawFinder(matrix: (boolean | null)[][], isFunc: boolean[][], row: number, col: number) {
  const size = matrix.length;
  for (let r = -1; r <= 7; r++) {
    for (let c = -1; c <= 7; c++) {
      const rr = row + r;
      const cc = col + c;
      if (rr < 0 || rr >= size || cc < 0 || cc >= size) continue;
      const inOuter = r === 0 || r === 6 || c === 0 || c === 6;
      const inInner = r >= 2 && r <= 4 && c >= 2 && c <= 4;
      const inSep = r === -1 || r === 7 || c === -1 || c === 7;
      matrix[rr][cc] = !inSep && (inOuter || inInner);
      isFunc[rr][cc] = true;
    }
  }
}

function drawAlignment(matrix: (boolean | null)[][], isFunc: boolean[][], row: number, col: number) {
  for (let r = -2; r <= 2; r++) {
    for (let c = -2; c <= 2; c++) {
      const dark = Math.abs(r) === 2 || Math.abs(c) === 2 || (r === 0 && c === 0);
      matrix[row + r][col + c] = dark;
      isFunc[row + r][col + c] = true;
    }
  }
}

function drawFormatBits(matrix: boolean[][], maskPattern: number) {
  const size = matrix.length;
  // Level M = 0b00
  const data = (0b00 << 3) | maskPattern;
  let rem = data << 10;
  for (let i = 4; i >= 0; i--) {
    if ((rem >>> (i + 10)) & 1) rem ^= 0x537 << i;
  }
  const bits = ((data << 10) | rem) ^ 0x5412;

  for (let i = 0; i < 15; i++) {
    const bit = ((bits >>> i) & 1) === 1;
    // Vertical + top-left / bottom-left
    if (i < 6) matrix[i][8] = bit;
    else if (i < 8) matrix[i + 1][8] = bit;
    else matrix[size - 15 + i][8] = bit;

    // Horizontal + top-right / top-left
    if (i < 8) matrix[8][size - 1 - i] = bit;
    else if (i < 9) matrix[8][15 - i] = bit;
    else matrix[8][14 - i] = bit;
  }
  matrix[size - 8][8] = true; // Dark module
}

function drawVersionBits(matrix: boolean[][], isFunc: boolean[][], version: number) {
  if (version < 7) return;
  const size = matrix.length;
  let rem = version << 12;
  for (let i = 5; i >= 0; i--) {
    if ((rem >>> (i + 12)) & 1) rem ^= 0x1f25 << i;
  }
  const bits = (version << 12) | rem;
  for (let i = 0; i < 18; i++) {
    const bit = ((bits >>> i) & 1) === 1;
    const a = Math.floor(i / 3);
    const b = i % 3;
    matrix[size - 11 + b][a] = bit;
    isFunc[size - 11 + b][a] = true;
    matrix[a][size - 11 + b] = bit;
    isFunc[a][size - 11 + b] = true;
  }
}

function applyMaskRule(mask: number, r: number, c: number): boolean {
  switch (mask) {
    case 0: return (r + c) % 2 === 0;
    case 1: return r % 2 === 0;
    case 2: return c % 3 === 0;
    case 3: return (r + c) % 3 === 0;
    case 4: return (Math.floor(r / 2) + Math.floor(c / 3)) % 2 === 0;
    case 5: return ((r * c) % 2) + ((r * c) % 3) === 0;
    case 6: return (((r * c) % 2) + ((r * c) % 3)) % 2 === 0;
    case 7: return (((r + c) % 2) + ((r * c) % 3)) % 2 === 0;
    default: return false;
  }
}

function penaltyScore(m: boolean[][]): number {
  const n = m.length;
  let score = 0;

  // Rule 1: 5+ consecutive same color in row/col
  for (let r = 0; r < n; r++) {
    let runColor = m[r][0];
    let runLen = 1;
    for (let c = 1; c < n; c++) {
      if (m[r][c] === runColor) {
        runLen++;
      } else {
        if (runLen >= 5) score += 3 + (runLen - 5);
        runColor = m[r][c];
        runLen = 1;
      }
    }
    if (runLen >= 5) score += 3 + (runLen - 5);
  }
  for (let c = 0; c < n; c++) {
    let runColor = m[0][c];
    let runLen = 1;
    for (let r = 1; r < n; r++) {
      if (m[r][c] === runColor) {
        runLen++;
      } else {
        if (runLen >= 5) score += 3 + (runLen - 5);
        runColor = m[r][c];
        runLen = 1;
      }
    }
    if (runLen >= 5) score += 3 + (runLen - 5);
  }

  // Rule 2: 2x2 blocks
  for (let r = 0; r < n - 1; r++) {
    for (let c = 0; c < n - 1; c++) {
      const v = m[r][c];
      if (v === m[r + 1][c] && v === m[r][c + 1] && v === m[r + 1][c + 1]) {
        score += 3;
      }
    }
  }

  // Rule 4: Dark balance
  let dark = 0;
  for (let r = 0; r < n; r++) {
    for (let c = 0; c < n; c++) if (m[r][c]) dark++;
  }
  const pct = (dark * 100) / (n * n);
  const k = Math.min(Math.abs(Math.floor(pct / 5) * 5 - 50), Math.abs(Math.ceil(pct / 5) * 5 - 50)) / 5;
  score += k * 10;

  return score;
}

/**
 * Encodes a UTF-8 string into a square QR Code boolean matrix (`true` = dark module).
 */
export function encodeQR(text: string): boolean[][] {
  const bytes = new TextEncoder().encode(text);
  const { spec, codewords } = selectSpecAndEncode(bytes);
  const size = spec.version * 4 + 17;

  const base: (boolean | null)[][] = Array.from({ length: size }, () => Array(size).fill(null));
  const isFunc: boolean[][] = Array.from({ length: size }, () => Array(size).fill(false));

  drawFinder(base, isFunc, 0, 0);
  drawFinder(base, isFunc, 0, size - 7);
  drawFinder(base, isFunc, size - 7, 0);

  // Timing patterns
  for (let i = 8; i < size - 8; i++) {
    const bit = i % 2 === 0;
    base[6][i] = bit;
    isFunc[6][i] = true;
    base[i][6] = bit;
    isFunc[i][6] = true;
  }

  // Alignment patterns
  const coords = ALIGNMENT_COORDS[spec.version];
  for (const r of coords) {
    for (const c of coords) {
      if (isFunc[r][c]) continue;
      drawAlignment(base, isFunc, r, c);
    }
  }

  // Reserve format info coordinates
  for (let i = 0; i < 9; i++) {
    if (i !== 6) {
      isFunc[8][i] = true;
      isFunc[i][8] = true;
    }
  }
  for (let i = size - 8; i < size; i++) {
    isFunc[8][i] = true;
    isFunc[i][8] = true;
  }
  isFunc[size - 8][8] = true;

  // Version info
  const seeded = base.map((row) => row.map((cell) => Boolean(cell)));
  drawVersionBits(seeded, isFunc, spec.version);

  // Place data + EC bits in zig-zag order
  const dataBits: boolean[] = [];
  for (const cw of codewords) {
    for (let b = 7; b >= 0; b--) dataBits.push(((cw >>> b) & 1) === 1);
  }

  let bitIdx = 0;
  let upward = true;
  for (let right = size - 1; right >= 1; right -= 2) {
    if (right === 6) right = 5;
    for (let vert = 0; vert < size; vert++) {
      const r = upward ? size - 1 - vert : vert;
      for (let lr = 0; lr < 2; lr++) {
        const c = right - lr;
        if (!isFunc[r][c]) {
          seeded[r][c] = bitIdx < dataBits.length ? dataBits[bitIdx++] : false;
        }
      }
    }
    upward = !upward;
  }

  // Choose lowest-penalty mask
  let bestMatrix = seeded;
  let bestScore = Infinity;
  for (let mask = 0; mask < 8; mask++) {
    const candidate = seeded.map((row, r) =>
      row.map((val, c) => (isFunc[r][c] ? val : applyMaskRule(mask, r, c) ? !val : val)),
    );
    drawFormatBits(candidate, mask);
    const s = penaltyScore(candidate);
    if (s < bestScore) {
      bestScore = s;
      bestMatrix = candidate;
    }
  }

  return bestMatrix;
}
