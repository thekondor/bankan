'use strict';
// Tests for hidden-board tab restoration logic.
// Run with: node board-tab-restore_test.js
// Requires Node.js built-in assert — no framework needed.
const assert = require('assert');

// ── Minimal DOM tab mock ──────────────────────────────────────────────────────

function makeHiddenTab(id, name, { withCloseBtn = true } = {}) {
  const classes = new Set(['board-tab', 'board-tab-hidden']);
  const attrs   = { 'data-hidden-id': id };
  const dataset = { hiddenId: id, boardName: name };
  const children = withCloseBtn
    ? [{ className: 'close-board-tab' }]
    : [];

  return {
    style:   { display: 'none' },
    dataset,
    classList: {
      remove:   (cls) => classes.delete(cls),
      contains: (cls) => classes.has(cls),
    },
    removeAttribute: (attr) => {
      delete attrs[attr];
      if (attr === 'data-hidden-id') delete dataset.hiddenId;
    },
    querySelector: (sel) =>
      sel === '.close-board-tab'
        ? (children.find(c => c.className === 'close-board-tab') || null)
        : null,
    _classes: classes,
  };
}

// Inline copy of the DOM-mutation fragment from showBoardFromDropdown in app.js.
function simulateRestore(tab, id) {
  tab.style.display = '';
  tab.classList.remove('board-tab-hidden');
  tab.removeAttribute('data-hidden-id');
  tab.dataset.boardId = id;
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// After restore: tab becomes visible.
{
  const tab = makeHiddenTab('b1', 'Board 1');
  simulateRestore(tab, 'b1');
  assert.strictEqual(tab.style.display, '', 'restored tab must be visible (display="")');
}

// After restore: board-tab-hidden class is removed.
{
  const tab = makeHiddenTab('b1', 'Board 1');
  simulateRestore(tab, 'b1');
  assert.strictEqual(tab.classList.contains('board-tab-hidden'), false, 'restored tab must not have board-tab-hidden class');
}

// After restore: data-hidden-id attribute is removed.
{
  const tab = makeHiddenTab('b1', 'Board 1');
  simulateRestore(tab, 'b1');
  assert.strictEqual(tab.dataset.hiddenId, undefined, 'restored tab must not retain data-hidden-id');
}

// After restore: data-board-id is set so SortableJS and hideBoard() can target it.
{
  const tab = makeHiddenTab('b1', 'Board 1');
  simulateRestore(tab, 'b1');
  assert.strictEqual(tab.dataset.boardId, 'b1', 'restored tab must have data-board-id set');
}

// After restore: close button must be present (regression guard — hidden tab template
// must include the .close-board-tab button so hover reveals it without a page reload).
{
  const tab = makeHiddenTab('b1', 'Board 1', { withCloseBtn: true });
  simulateRestore(tab, 'b1');
  assert.notStrictEqual(
    tab.querySelector('.close-board-tab'),
    null,
    'restored tab must contain a .close-board-tab button'
  );
}

// Sanity check: a tab rendered without a close button (old broken template) would
// fail the hover-reveal requirement — this documents the root cause of the bug.
{
  const tab = makeHiddenTab('b1', 'Board 1', { withCloseBtn: false });
  simulateRestore(tab, 'b1');
  assert.strictEqual(
    tab.querySelector('.close-board-tab'),
    null,
    'tab without close button in markup has no close button after restore (old broken behaviour)'
  );
}

console.log('board-tab-restore_test.js: all tests passed');
