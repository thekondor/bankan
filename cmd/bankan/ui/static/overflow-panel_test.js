'use strict';
// Tests for overflow dropdown panel logic and hide-board navigation resolution.
// Run with: node overflow-panel_test.js
// Requires Node.js built-in assert — no framework needed.
const assert = require('assert');

// ── _shouldShowDropdownEmptyMsg ───────────────────────────────────────────────

function _shouldShowDropdownEmptyMsg(hasHiddenItems, anyArchivedInDropdown) {
  return !hasHiddenItems && !anyArchivedInDropdown;
}

assert.strictEqual(_shouldShowDropdownEmptyMsg(false, false), true,  'empty panel → show placeholder');
assert.strictEqual(_shouldShowDropdownEmptyMsg(true,  false), false, 'has hidden items → hide placeholder');
assert.strictEqual(_shouldShowDropdownEmptyMsg(false, true),  false, 'has archived in dropdown → hide placeholder');
assert.strictEqual(_shouldShowDropdownEmptyMsg(true,  true),  false, 'has both → hide placeholder');

// ── _resolveHideActiveTabNavigation ──────────────────────────────────────────

function _resolveHideActiveTabNavigation(serverTarget, firstArchivedTabHref) {
  if (firstArchivedTabHref && /^\/ui\/workspaces\/[^\/]+$/.test(serverTarget)) {
    return firstArchivedTabHref;
  }
  return serverTarget;
}

const wsRoot     = '/ui/workspaces/myws';
const wsBoard    = '/ui/workspaces/myws/boards/b1';
const archBoard  = '/ui/workspaces/myws/boards/archived1';

// Server sent workspace root + archived tab visible → navigate to archived board.
assert.strictEqual(
  _resolveHideActiveTabNavigation(wsRoot, archBoard),
  archBoard,
  'workspace root + visible archived tab → use archived tab href'
);

// Server sent a specific board (still has active boards) → keep server target.
assert.strictEqual(
  _resolveHideActiveTabNavigation(wsBoard, archBoard),
  wsBoard,
  'server already has a board target → do not override with archived tab'
);

// Server sent workspace root but no archived tab visible → keep workspace root.
assert.strictEqual(
  _resolveHideActiveTabNavigation(wsRoot, null),
  wsRoot,
  'workspace root + no archived tab → navigate to workspace root as-is'
);

// Server sent '/' fallback and archived tab visible → '/' is not a workspace root, keep it.
assert.strictEqual(
  _resolveHideActiveTabNavigation('/', archBoard),
  '/',
  '"/" is not a workspace root pattern → do not override'
);

// Workspace ID with hyphens and numbers in path.
assert.strictEqual(
  _resolveHideActiveTabNavigation('/ui/workspaces/product-foo-42', archBoard),
  archBoard,
  'workspace root with hyphens/numbers → use archived tab href'
);

console.log('overflow-panel_test.js: all tests passed');
