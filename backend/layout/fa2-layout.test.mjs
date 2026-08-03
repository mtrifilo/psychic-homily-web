// Tests for the ForceAtlas2 layout worker. Run with `node --test` from
// backend/layout (no test framework is installed — `node --test` ships with the
// runtime, and the Go side must not grow a JS toolchain).

import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {dirname, join} from 'node:path';
import test from 'node:test';

import {runLayout} from './fa2-layout.mjs';

const scriptPath = join(dirname(fileURLToPath(import.meta.url)), 'fa2-layout.mjs');

// ring builds a deterministic n-cycle with a fixed, non-degenerate start
// layout. Every node sits on a circle, so no two nodes share a position (FA2
// repulsion is undefined at zero distance) and the input is reproducible
// without an RNG.
function ring(n) {
  const nodes = [];
  const edges = [];
  for (let i = 0; i < n; i++) {
    const t = (2 * Math.PI * i) / n;
    nodes.push({x: Math.cos(t) * 10, y: Math.sin(t) * 10, fixed: false});
    edges.push({source: i, target: (i + 1) % n, weight: 1});
  }
  return {iterations: 50, nodes, edges};
}

test('identical input produces byte-identical output', () => {
  const request = ring(120);
  const a = JSON.stringify(runLayout(structuredClone(request)));
  const b = JSON.stringify(runLayout(structuredClone(request)));
  assert.equal(a, b);
});

test('a fixed node keeps byte-identical coordinates through assign()', () => {
  // `fixed` is an UNDOCUMENTED node attribute of
  // graphology-layout-forceatlas2@0.10.1: helpers.js copies `attr.fixed` into
  // NodeMatrix slot 9 and iterate.js gates both displacement writes on
  // `NodeMatrix[n + NODE_FIXED] !== 1`. The pipeline relies on it to pin
  // anchors, so this test is the tripwire for a version bump silently dropping
  // the attribute — which would look like a layout that merely drifted more
  // than usual.
  //
  // "Byte-identical" is asserted against Math.fround of the input, not the
  // input: NodeMatrix is a Float32Array, so even an untouched node's
  // coordinates come back float32-rounded. Feeding already-float32 values in
  // (which the Go caller does) makes this an exact identity.
  const request = ring(60);
  request.nodes[0].fixed = true;
  request.nodes[7].fixed = true;
  const before = request.nodes.map((n) => [Math.fround(n.x), Math.fround(n.y)]);

  const {positions} = runLayout(structuredClone(request));

  assert.deepEqual(positions[0], before[0]);
  assert.deepEqual(positions[7], before[7]);
  // Sanity: the unpinned nodes DID move, so the assertion above is testing the
  // pin and not an iteration count of zero.
  assert.notDeepEqual(positions[3], before[3]);
});

test('float32 input round-trips exactly through a fixed node', () => {
  // The Go caller's warm start replays stored float32 positions. If those
  // survive assign() unchanged, an unchanged graph reproduces byte-identical
  // output — the whole stability contract in one assertion.
  const request = ring(24);
  for (const n of request.nodes) {
    n.x = Math.fround(n.x);
    n.y = Math.fround(n.y);
    n.fixed = true;
  }
  const before = request.nodes.map((n) => [n.x, n.y]);
  const {positions} = runLayout(structuredClone(request));
  assert.deepEqual(positions, before);
});

test('an isolated node is laid out without an edge', () => {
  const request = ring(10);
  request.nodes.push({x: 3, y: 4, fixed: false});
  const {positions} = runLayout(structuredClone(request));
  assert.equal(positions.length, 11);
  assert.ok(Number.isFinite(positions[10][0]));
});

test('malformed requests are rejected rather than silently degraded', () => {
  assert.throws(() => runLayout({nodes: []}), /non-empty array/);
  assert.throws(
    () => runLayout({nodes: [{x: 0, y: 0}], edges: [{source: 0, target: 5}]}),
    /out-of-range target/,
  );
  assert.throws(
    () => runLayout({nodes: [{x: 0, y: 0}, {x: 1, y: 1}], edges: [{source: 0, target: 0}]}),
    /self-loop/,
  );
  assert.throws(() => runLayout({nodes: [{x: 0, y: Number.NaN}]}), /finite x and y/);
  assert.throws(() => runLayout({nodes: [{x: 0, y: 0}], iterations: 0}), /iterations/);
});

test('the stdin/stdout process contract round-trips', () => {
  const request = ring(30);
  const out = execFileSync('node', [scriptPath], {input: JSON.stringify(request)});
  const parsed = JSON.parse(out.toString('utf8'));
  assert.equal(parsed.positions.length, 30);
  // The process path must agree with the in-process path exactly — the Go
  // caller consumes the former and this file tests the latter.
  assert.equal(out.toString('utf8'), JSON.stringify(runLayout(structuredClone(request))));
});

test('a bad request exits non-zero and writes nothing to stdout', () => {
  assert.throws(
    () => execFileSync('node', [scriptPath], {input: '{"nodes":[]}', stdio: 'pipe'}),
    (err) => {
      assert.equal(err.status, 1);
      assert.equal(err.stdout.toString('utf8'), '');
      assert.match(err.stderr.toString('utf8'), /non-empty array/);
      return true;
    },
  );
});
