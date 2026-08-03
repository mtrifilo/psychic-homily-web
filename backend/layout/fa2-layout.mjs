#!/usr/bin/env node
// ForceAtlas2 layout worker for the nightly graph-overview snapshot.
//
// The Go job (internal/services/catalog/graph_overview_layout.go) invokes this
// script as a subprocess, writes ONE JSON request on stdin, and reads ONE JSON
// response from stdout. Nothing else may be written to stdout — diagnostics go
// to stderr, which the Go side logs.
//
// WHY A SUBPROCESS: there is no maintained pure-Go ForceAtlas2 with Barnes-Hut
// approximation, and the client-side warmup this snapshot replaces costs
// seconds of blocked main thread. graphology's implementation is the measured
// reference the layout parameters were tuned against, so the pipeline runs it
// rather than re-deriving a second layout's behaviour.
//
// THIS SCRIPT IS A PURE FUNCTION of its request. It does no I/O beyond
// stdin/stdout, holds no state between runs, and contains no RNG — so identical
// input yields byte-identical output. Everything that needs history (warm-start
// positions, barycenter seeding of new nodes, Procrustes alignment to the
// previous epoch) is the Go caller's job; keeping it there is what makes this
// side testable as a deterministic transform.
//
// Request:
//   {
//     "iterations": 50,
//     "nodes": [{"x": 1.5, "y": -0.5, "fixed": false}, ...],
//     "edges": [{"source": 0, "target": 3, "weight": 1.0}, ...]
//   }
// The physics settings are NOT part of the request — see the frozen `settings`
// object below for why they cannot be.
// Node identity is the ARRAY INDEX — `edges[].source`/`target` are indexes into
// `nodes`, and the response's positions come back in the same order. Ids never
// cross the boundary: the layout does not care what a node is, and keeping the
// wire format index-based keeps a 5k-node request small and its parse cheap.
//
// Response:
//   {"positions": [[x, y], ...]}   // same order and length as request.nodes
//
// PRECISION IS FLOAT32, NOT FLOAT64. graphology-layout-forceatlas2@0.10.1 copies
// every coordinate into a Float32Array (helpers.js `new Float32Array(order *
// PPN)`) and reads them back out, so EVERY returned coordinate — including a
// `fixed` node's, which the iteration never touches — is the float32 rounding of
// what went in. The Go caller therefore stores positions as float32; feeding
// float32 values back in on the next epoch is then an exact round trip, which is
// what makes "re-run on unchanged data reproduces byte-identical positions"
// true. Handing this script a float64 coordinate is not an error, it just gets
// rounded, and the resulting output would differ from a run seeded with the
// rounded value.

import {UndirectedGraph} from 'graphology';
import forceAtlas2 from 'graphology-layout-forceatlas2';

// maxIterations bounds a malformed or runaway request. The pipeline asks for
// ~50 (the stability-vs-quality tradeoff measured for this map); anything past
// this is a bug in the caller, not a legitimate ask.
const MAX_ITERATIONS = 10000;

function fail(message) {
  process.stderr.write(`fa2-layout: ${message}\n`);
  process.exit(1);
}

function readStdin() {
  return new Promise((resolve, reject) => {
    const chunks = [];
    process.stdin.on('data', (c) => chunks.push(c));
    process.stdin.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
    process.stdin.on('error', reject);
  });
}

// runLayout is the whole contract, exported so the test file can drive it
// without spawning a process. It mutates nothing the caller owns.
export function runLayout(request) {
  if (!request || typeof request !== 'object') {
    throw new Error('request must be a JSON object');
  }
  const nodes = request.nodes;
  if (!Array.isArray(nodes) || nodes.length === 0) {
    throw new Error('request.nodes must be a non-empty array');
  }
  const edges = Array.isArray(request.edges) ? request.edges : [];

  // The library itself refuses a non-positive count ("you should provide a
  // positive number of iterations"), so the floor is 1 rather than 0.
  const iterations = request.iterations ?? 50;
  if (!Number.isInteger(iterations) || iterations < 1 || iterations > MAX_ITERATIONS) {
    throw new Error(`request.iterations must be an integer in [1, ${MAX_ITERATIONS}]`);
  }

  const graph = new UndirectedGraph();
  // Insertion order IS the node order graphology iterates in, and FA2 sums
  // forces in that order. Float addition is not associative, so a different
  // insertion order changes results at the ULP level and can flip the layout
  // over many iterations — the caller sorts by a stable key and this loop
  // preserves it verbatim.
  for (let i = 0; i < nodes.length; i++) {
    const n = nodes[i];
    if (!Number.isFinite(n?.x) || !Number.isFinite(n?.y)) {
      throw new Error(`node ${i} needs finite x and y`);
    }
    graph.addNode(String(i), {x: n.x, y: n.y, fixed: n.fixed === true});
  }

  for (let i = 0; i < edges.length; i++) {
    const e = edges[i];
    const {source, target} = e ?? {};
    if (!Number.isInteger(source) || source < 0 || source >= nodes.length) {
      throw new Error(`edge ${i} has out-of-range source ${source}`);
    }
    if (!Number.isInteger(target) || target < 0 || target >= nodes.length) {
      throw new Error(`edge ${i} has out-of-range target ${target}`);
    }
    if (source === target) {
      throw new Error(`edge ${i} is a self-loop on node ${source}`);
    }
    const weight = e.weight ?? 1;
    if (!Number.isFinite(weight) || weight <= 0) {
      throw new Error(`edge ${i} needs a finite positive weight`);
    }
    // addUndirectedEdge THROWS on a duplicate pair rather than merging. That is
    // deliberate: the caller collapses parallel edges before it gets here, so a
    // duplicate means the edge set disagrees with the node set and the layout
    // would silently double that pair's attraction.
    graph.addUndirectedEdge(String(source), String(target), {weight});
  }

  // Settings are FROZEN here and deliberately not overridable per request.
  // forceAtlas2.inferSettings() derives scalingRatio and barnesHutOptimize FROM
  // GRAPH ORDER, so a snapshot that grew by one node could silently switch
  // physics between epochs — the exact reshuffle the stability contract exists
  // to prevent. Letting a caller pass settings would reintroduce the same
  // hazard by a different route, so it cannot.
  //
  // The values below are forceAtlas2.inferSettings()'s recommendations
  // EVALUATED ONCE at this map's scale (~5k nodes) and then frozen. Freezing
  // matters twice over: it removes the order dependency, and the un-damped
  // library defaults (gravity 1, slowDown 1, weak gravity) let a graph with
  // long-range edges expand without bound — measured at 5k/20k, that
  // configuration had not finished 50 Barnes-Hut iterations after two minutes,
  // versus 1.6s with these.
  const settings = {
    barnesHutOptimize: true,
    barnesHutTheta: 0.5,
    strongGravityMode: true,
    gravity: 0.05,
    scalingRatio: 10,
    slowDown: 10,
    linLogMode: false,
    outboundAttractionDistribution: false,
    adjustSizes: false,
    edgeWeightInfluence: 1,
  };

  forceAtlas2.assign(graph, {iterations, settings, getEdgeWeight: 'weight'});

  const positions = new Array(nodes.length);
  graph.forEachNode((key, attr) => {
    positions[Number(key)] = [attr.x, attr.y];
  });
  for (let i = 0; i < positions.length; i++) {
    if (!positions[i] || !Number.isFinite(positions[i][0]) || !Number.isFinite(positions[i][1])) {
      // A non-finite coordinate means the physics diverged. Failing here keeps
      // the previous snapshot live instead of persisting a map with NaN nodes,
      // which no downstream consumer can detect once quantized.
      throw new Error(`node ${i} ended at a non-finite position`);
    }
  }
  return {positions};
}

async function main() {
  const raw = await readStdin();
  let request;
  try {
    request = JSON.parse(raw);
  } catch (err) {
    fail(`could not parse stdin as JSON: ${err.message}`);
  }
  let result;
  try {
    result = runLayout(request);
  } catch (err) {
    fail(err.message);
  }
  process.stdout.write(JSON.stringify(result));
}

// Only run the stdin pump when executed as a script; importing for tests must
// not block on a stdin that never closes.
if (process.argv[1] && import.meta.url === `file://${process.argv[1]}`) {
  main().catch((err) => fail(err.stack || String(err)));
}
