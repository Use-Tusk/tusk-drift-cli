#!/usr/bin/env node
/**
 * Processes raw V8 coverage files and outputs per-file statement counts.
 * Usage: node process-v8-coverage.js <v8-coverage-file.json> [source-root]
 *
 * Outputs JSON: { "/path/to/file.js": { "statements": { "lineNumber": hitCount } } }
 * Only includes user source files (excludes node_modules, node: builtins).
 */

const fs = require('fs');
const path = require('path');

const v8File = process.argv[2];
const sourceRoot = process.argv[3] || process.cwd();

if (!v8File) {
  console.error('Usage: node process-v8-coverage.js <v8-coverage-file.json> [source-root]');
  process.exit(1);
}

const data = JSON.parse(fs.readFileSync(v8File, 'utf-8'));
const result = {};

for (const script of data.result) {
  // Skip non-file URLs (node: builtins, eval, etc.)
  if (!script.url.startsWith('file://')) continue;

  const filePath = script.url.replace('file://', '');

  // Skip node_modules
  if (filePath.includes('node_modules')) continue;

  // Skip files outside source root
  if (!filePath.startsWith(sourceRoot)) continue;

  // Read source file to map byte offsets to line numbers
  let source;
  try {
    source = fs.readFileSync(filePath, 'utf-8');
  } catch {
    continue;
  }

  // Build offset-to-line mapping
  const lineStarts = [0];
  for (let i = 0; i < source.length; i++) {
    if (source[i] === '\n') {
      lineStarts.push(i + 1);
    }
  }

  function offsetToLine(offset) {
    let lo = 0, hi = lineStarts.length - 1;
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1;
      if (lineStarts[mid] <= offset) lo = mid;
      else hi = mid - 1;
    }
    return lo + 1; // 1-based
  }

  const lineCounts = {};

  for (const func of script.functions) {
    for (const range of func.ranges) {
      if (range.count === 0) continue;

      const startLine = offsetToLine(range.startOffset);
      const endLine = offsetToLine(range.endOffset);

      for (let line = startLine; line <= endLine; line++) {
        lineCounts[line] = (lineCounts[line] || 0) + range.count;
      }
    }
  }

  if (Object.keys(lineCounts).length > 0) {
    result[filePath] = { lines: lineCounts };
  }
}

process.stdout.write(JSON.stringify(result));
