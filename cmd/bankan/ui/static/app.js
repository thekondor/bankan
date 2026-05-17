/* bankan UI application JavaScript */

// ── Token management ──────────────────────────────────────────────────────
function getToken() {
  const meta = document.querySelector('meta[name="bankan-token"]');
  return meta ? meta.content : '';
}

// ── HTMX token injection ──────────────────────────────────────────────────
document.addEventListener('htmx:configRequest', function(evt) {
  const token = getToken();
  if (token) {
    evt.detail.headers['X-Bankan-Token'] = token;
  }
});

// ── HTMX response handling ────────────────────────────────────────────────
document.addEventListener('htmx:responseError', function(evt) {
  let msg = 'Request failed';
  try {
    const json = JSON.parse(evt.detail.xhr.responseText);
    if (json.error) msg = json.error;
  } catch (_) {}
  showToast(msg, 'error');
});

// ── Dropdown toggling ─────────────────────────────────────────────────────
function toggleDropdown(btn) {
  const menu = btn.nextElementSibling;
  const wasOpen = menu.classList.contains('open');
  closeAllDropdowns();
  if (!wasOpen) {
    menu.classList.add('open');
  }
}

function closeAllDropdowns() {
  document.querySelectorAll('.dropdown-menu.open').forEach(m => m.classList.remove('open'));
}

function closeAllCardLabelPickers() {
  document.querySelectorAll('.card-label-picker-menu').forEach(m => m.style.display = 'none');
}

document.addEventListener('click', function(e) {
  if (!e.target.closest('.dropdown') && !e.target.closest('.card-label-wrap')) {
    closeAllDropdowns();
    closeAllCardLabelPickers();
  }
});

// ── Modal helpers ─────────────────────────────────────────────────────────

// Returns true if a card-related modal is currently open in #modal-target.
// Used to decide whether closing the modal should also clear ?card from the URL.
function _isCardModalOpen() {
  var ids = ['card-modal', 'move-card-modal', 'edit-card-modal', 'unarchive-card-modal'];
  return ids.some(function(id) { return !!document.getElementById(id); });
}

// Parses the board ID out of the current pathname (/ui/boards/{id}/...).
function _extractBoardIdFromPath() {
  var m = location.pathname.match(/\/ui\/boards\/([^\/]+)/);
  return m ? m[1] : null;
}

// Removes ?card and ?comment from the URL without navigating.
function _clearCardFromUrl() {
  var params = new URLSearchParams(location.search);
  if (!params.has('card') && !params.has('comment')) return;
  params.delete('card');
  params.delete('comment');
  var search = params.toString();
  history.pushState(null, '', location.pathname + (search ? '?' + search : ''));
}

// Adds (or replaces) ?card in the URL, removing any stale ?comment.
function _pushCardUrl(cardID) {
  var params = new URLSearchParams(location.search);
  params.set('card', cardID);
  params.delete('comment');
  history.pushState({cardId: cardID}, '', location.pathname + '?' + params.toString());
}

// Closes #modal-target. If a card modal was open, also clears ?card/?comment from URL.
function closeModal() {
  var wasCard = _isCardModalOpen();
  document.getElementById('modal-target').innerHTML = '';
  if (wasCard) {
    _clearCardFromUrl();
  }
}

function closeModalOnBackdrop(event) {
  if (event.target.classList.contains('modal-backdrop')) {
    closeModal();
  }
}

function isMenuClick(event) {
  return !!event.target.closest('[data-menu="true"]');
}

// ── Add lane form ─────────────────────────────────────────────────────────
function openAddLaneForm(btn) {
  const form = document.getElementById('add-lane-form');
  if (form) {
    form.classList.add('open');
    btn.style.display = 'none';
    form.querySelector('input').focus();
  }
}

function closeAddLaneForm() {
  const form = document.getElementById('add-lane-form');
  if (form) {
    form.classList.remove('open');
    const btn = form.previousElementSibling;
    if (btn) btn.style.display = '';
    form.querySelector('input').value = '';
  }
}

// Core fetch logic for adding a lane — called from the click handler and
// the Enter-key handler on the input.
function doSubmitAddLane(boardID) {
  const form = document.getElementById('add-lane-form');
  const name = form.querySelector('input').value.trim();
  if (!name) return;
  fetch(`/ui/boards/${boardID}/lanes`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': getToken() },
    body: JSON.stringify({ name })
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    closeAddLaneForm();
    reloadBoard(boardID);
  }).catch(e => showToast(e.message, 'error'));
}

// Submit add-lane via click on the Add Lane button.
document.addEventListener('click', function(e) {
  const btn = e.target.closest('[data-action="add-lane-submit"]');
  if (!btn) return;
  e.preventDefault();
  doSubmitAddLane(btn.dataset.board);
});

// ── Add card form ─────────────────────────────────────────────────────────
function openAddCardForm(laneName, boardID) {
  closeAllAddCardForms();
  const formId = 'add-card-form-' + laneName;
  const form = document.getElementById(formId);
  if (form) {
    form.classList.add('open');
    form.querySelector('input[type=text]').focus();
  }
}

function closeAddCardForm(laneName) {
  const form = document.getElementById('add-card-form-' + laneName);
  if (form) {
    form.classList.remove('open');
    form.querySelectorAll('input[type=text], textarea').forEach(el => el.value = '');
    form.querySelectorAll('input[type=checkbox]').forEach(el => { el.checked = false; });
  }
}

function closeAllAddCardForms() {
  document.querySelectorAll('.inline-form.open').forEach(f => {
    if (f.id && f.id.startsWith('add-card-form-')) {
      f.classList.remove('open');
      f.querySelectorAll('input[type=text], textarea').forEach(el => el.value = '');
      f.querySelectorAll('input[type=checkbox]').forEach(el => { el.checked = false; });
    }
  });
}

function submitAddCard(laneName, boardID, btn) {
  const form  = document.getElementById('add-card-form-' + laneName);
  const title = document.getElementById('new-card-title-' + laneName)?.value.trim() || '';
  const body  = document.getElementById('new-card-body-' + laneName)?.value.trim()  || '';
  const labelIDs = Array.from(form.querySelectorAll('input[name=labels]:checked')).map(i => i.value);

  if (!title) return;

  fetch(`/ui/boards/${boardID}/cards`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': getToken() },
    body: JSON.stringify({ lane: laneName, title, body, label_ids: labelIDs })
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    return r.text();
  }).then(html => {
    const cardsEl = document.getElementById('cards-' + laneName);
    if (cardsEl) {
      const empty = cardsEl.querySelector('.empty-lane');
      if (empty) empty.remove();
      cardsEl.insertAdjacentHTML('beforeend', html);
      // Update count badge
      updateLaneCount(laneName);
    }
    closeAddCardForm(laneName);
    initSortable();
  }).catch(e => showToast(e.message, 'error'));
}

// ── Card board-view label picker ──────────────────────────────────────────
let _boardPickerCoords = null;

function toggleCardBoardLabelPicker(event, btn) {
  event.stopPropagation();
  const menu = btn.nextElementSibling;
  const wasOpen = menu.style.display !== 'none';
  closeAllCardLabelPickers();
  closeAllDropdowns();
  if (!wasOpen) {
    const rect = btn.getBoundingClientRect();
    _boardPickerCoords = { top: rect.bottom + 4, right: window.innerWidth - rect.right };
    _showBoardPicker(menu);
  } else {
    _boardPickerCoords = null;
  }
}

function _showBoardPicker(menu) {
  if (!_boardPickerCoords) return;
  // Reset checkbox states to what the server rendered, overriding any browser
  // form-state restoration that may have applied stale checked values.
  menu.querySelectorAll('input[type=checkbox]').forEach(cb => { cb.checked = cb.defaultChecked; });
  menu.style.top   = _boardPickerCoords.top   + 'px';
  menu.style.right = _boardPickerCoords.right + 'px';
  menu.style.display = 'flex';
}

// ── Archive card ──────────────────────────────────────────────────────────
function archiveCard(cardID, boardID) {
  // When show_archived is active the server returns a full board view so the
  // card re-appears as archived in its lane. Use htmx.ajax for the swap.
  if (new URLSearchParams(window.location.search).get('show_archived') === 'true') {
    closeModal();
    htmx.ajax('POST', `/ui/boards/${boardID}/cards/${cardID}/archive`, {
      target: '#board-view',
      swap: 'outerHTML',
      headers: { 'X-Bankan-Token': getToken() }
    });
    return;
  }

  const cardEl = document.getElementById('card-' + cardID);
  const cardsEl = cardEl ? cardEl.closest('.lane-cards') : null;
  const laneName = cardsEl ? cardsEl.dataset.lane : null;

  fetch(`/ui/boards/${boardID}/cards/${cardID}/archive`, {
    method: 'POST',
    headers: { 'X-Bankan-Token': getToken() }
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    if (cardEl) cardEl.remove();
    closeModal();
    if (laneName) updateLaneCount(laneName);
  }).catch(e => showToast(e.message, 'error'));
}

// ── Duplicate card ────────────────────────────────────────────────────────
function duplicateCard(cardID, boardID) {
  closeAllDropdowns();
  fetch(`/ui/boards/${boardID}/cards/${cardID}/duplicate`, {
    method: 'POST',
    headers: { 'X-Bankan-Token': getToken() }
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    const newCardID = r.headers.get('X-New-Card-ID');
    return r.text().then(html => ({ html, newCardID }));
  }).then(({ html, newCardID }) => {
    // Update the board view so the new card is already visible in its lane.
    const boardEl = document.getElementById('board-view');
    if (boardEl) boardEl.outerHTML = html;
    // Open the edit modal for the new card.
    if (newCardID) openEditCardModal(newCardID, boardID);
  }).catch(e => showToast(e.message, 'error'));
}

// ── Card detail modal ─────────────────────────────────────────────────────
function openCardDetailModal(event, cardID, boardID) {
  if (event && isMenuClick(event)) return;
  return fetch(`/ui/modals/card/${cardID}/boards/${boardID}`, {
    headers: { 'X-Bankan-Token': getToken() }
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    return r.text();
  }).then(html => {
    document.getElementById('modal-target').innerHTML = html;
    _pushCardUrl(cardID);
  }).catch(e => showToast(e.message, 'error'));
}

// Navigates to baseUrl, appending ?comment= from the current URL if present.
// Used by CardNotInViewModal so the comment anchor survives the redirect to the
// parent board.
function navigatePreservingComment(baseUrl) {
  var commentId = new URLSearchParams(location.search).get('comment');
  if (commentId) {
    baseUrl += '&comment=' + encodeURIComponent(commentId);
  }
  window.location.href = baseUrl;
}

// ── Card / comment permalink copy ─────────────────────────────────────────

// Copies the current page URL (which already includes ?card=) to the clipboard.
function copyCardLink() {
  navigator.clipboard.writeText(location.href).then(function() {
    showToast('Link copied to clipboard', 'success');
  }).catch(function() {
    showToast('Failed to copy link', 'error');
  });
}

// Builds a URL with ?card=... (current) and &comment={commentId}, then copies it.
function copyCommentLink(commentId) {
  var params = new URLSearchParams(location.search);
  params.set('comment', commentId);
  var url = location.origin + location.pathname + '?' + params.toString();
  navigator.clipboard.writeText(url).then(function() {
    showToast('Comment link copied to clipboard', 'success');
  }).catch(function() {
    showToast('Failed to copy link', 'error');
  });
}

// Toggles the inline label picker inside the card detail modal.
function toggleCardLabelPicker() {
  const picker = document.getElementById('card-label-picker');
  if (!picker) return;
  picker.style.display = picker.style.display === 'none' ? 'flex' : 'none';
}

// Handles a label pill click inside the card detail modal picker.
// Immediately PATCHes the label change, refreshes the board card and modal.
function onCardLabelClick(event, el) {
  event.preventDefault();
  event.stopPropagation();
  const checkbox = el.querySelector('input[type=checkbox]');
  const wasChecked = checkbox.checked;
  const cardID  = el.dataset.card;
  const boardID = el.dataset.board;
  const labelID = el.dataset.label;
  const payload = wasChecked ? { remove_labels: [labelID] } : { add_labels: [labelID] };

  // Use the UI PATCH endpoint: updates the card and returns the updated card HTML.
  fetch(`/ui/boards/${boardID}/cards/${cardID}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': getToken() },
    body: JSON.stringify(payload)
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    return r.text();
  }).then(cardHtml => {
    // Refresh the card on the board in-place.
    const boardCard = document.getElementById('card-' + cardID);
    if (boardCard) boardCard.outerHTML = cardHtml;
    // Re-open the board card picker only when the click originated from the
    // board view (not from the card detail modal), so the inline picker stays
    // open during rapid label changes.
    const clickFromModal = !!event.target.closest('#card-modal');
    if (!clickFromModal) {
      const newBoardCard = document.getElementById('card-' + cardID);
      if (newBoardCard) {
        const boardPicker = newBoardCard.querySelector('.card-label-picker-menu');
        if (boardPicker) _showBoardPicker(boardPicker);
      }
    }
    // If the card detail modal is open, re-fetch it and keep its picker open too.
    if (document.getElementById('card-modal')) {
      return fetch(`/ui/modals/card/${cardID}/boards/${boardID}`, {
        headers: { 'X-Bankan-Token': getToken() }
      }).then(r2 => r2.text()).then(html => {
        document.getElementById('modal-target').innerHTML = html;
        const modalPicker = document.getElementById('card-label-picker');
        if (modalPicker) modalPicker.style.display = 'flex';
      });
    }
  }).catch(e => showToast(e.message, 'error'));
}

// ── Move card modal ───────────────────────────────────────────────────────
function openMoveCardModal(cardID, boardID) {
  closeAllDropdowns();
  fetch(`/ui/modals/card/${cardID}/boards/${boardID}?view=move`, {
    headers: { 'X-Bankan-Token': getToken() }
  }).then(r => r.text()).then(html => {
    document.getElementById('modal-target').innerHTML = html;
  });
}

// Fires a move-card request via htmx.ajax so that HTMX request lifecycle
// (headers, history, swap) is handled correctly. The modal is cleared first
// so the DOM is clean before the board view is swapped.
function submitMoveCard(event, cardID, boardID, laneName) {
  if (event) event.stopPropagation();
  closeModal();
  htmx.ajax('POST', `/ui/boards/${boardID}/cards/${cardID}/move`, {
    target: '#board-view',
    swap: 'outerHTML',
    values: { to_lane: laneName },
    headers: { 'X-Bankan-Token': getToken() }
  });
}

// ── Edit card modal ───────────────────────────────────────────────────────
function openEditCardModal(cardID, boardID) {
  closeAllDropdowns();
  fetch(`/ui/modals/card/${cardID}/boards/${boardID}?view=edit`, {
    headers: { 'X-Bankan-Token': getToken() }
  }).then(r => r.text()).then(html => {
    document.getElementById('modal-target').innerHTML = html;
    const ta = document.querySelector('#edit-card-modal textarea[name="body"]');
    if (ta) { ta.style.height = 'auto'; ta.style.height = Math.max(ta.scrollHeight, 160) + 'px'; }
  });
}

// ── Unarchive card modal ──────────────────────────────────────────────────
// Opens the lane-picker modal for restoring an archived card.
function openUnarchiveCardModal(event, cardID, boardID) {
  if (event) event.stopPropagation();
  closeAllDropdowns();
  fetch(`/ui/modals/card/${cardID}/boards/${boardID}?view=unarchive`, {
    headers: { 'X-Bankan-Token': getToken() }
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    return r.text();
  }).then(html => {
    document.getElementById('modal-target').innerHTML = html;
  }).catch(e => showToast(e.message, 'error'));
}

// Fires a restore-card request via htmx.ajax so that HTMX request lifecycle
// (headers, history, swap) is handled correctly. The modal is cleared first
// so the DOM is clean before the board view is swapped.
function submitRestoreCard(event, cardID, boardID, laneName) {
  if (event) event.stopPropagation();
  closeModal();
  htmx.ajax('POST', `/ui/boards/${boardID}/cards/${cardID}/restore`, {
    target: '#board-view',
    swap: 'outerHTML',
    values: { to_lane: laneName },
    headers: { 'X-Bankan-Token': getToken() }
  });
}

function submitEditCard(event, form) {
  event.preventDefault();
  const boardID = form.dataset.board;
  const cardID  = form.dataset.card;
  const token   = form.dataset.token || getToken();
  const title   = form.querySelector('[name=title]').value.trim();
  const body    = form.querySelector('[name=body]').value.trim();
  const checked = Array.from(form.querySelectorAll('[name=labels]:checked')).map(c => c.value);
  const unchecked = Array.from(form.querySelectorAll('[name=labels]:not(:checked)')).map(c => c.value);
  const primaryLabelEl = form.querySelector('[name=primary_label]');
  const primaryLabel = primaryLabelEl ? primaryLabelEl.value : null;

  fetch(`/api/v1/boards/${boardID}/cards/${cardID}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': token },
    body: JSON.stringify({ title, body, add_labels: checked, remove_labels: unchecked, primary_label: primaryLabel })
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    return r.json();
  }).then(card => {
    closeModal();
    reloadBoard(boardID);
  }).catch(e => showToast(e.message, 'error'));
}

// ── Add board modal ───────────────────────────────────────────────────────
function openAddBoardModal() {
  const match = window.location.pathname.match(/^\/ui\/boards\/([^/]+)/);
  const qs = match ? '?board_id=' + encodeURIComponent(match[1]) : '';
  fetch('/ui/modals/add-board' + qs).then(r => r.text()).then(html => {
    document.getElementById('modal-target').innerHTML = html;
  });
}

// ── Color utilities ────────────────────────────────────────────────────────
function hslToHex(h, s, l) {
  l /= 100;
  const a = s * Math.min(l, 1 - l) / 100;
  const f = n => {
    const k = (n + h / 30) % 12;
    const c = l - a * Math.max(Math.min(k - 3, 9 - k, 1), -1);
    return Math.round(255 * c).toString(16).padStart(2, '0');
  };
  return `#${f(0)}${f(8)}${f(4)}`;
}

// Parse a CSS color string ("#rrggbb" or "rgb(r, g, b)") to {r, g, b}.
function parseCSSColor(str) {
  const s = str.trim();
  if (/^#[0-9a-fA-F]{6}$/.test(s)) {
    return { r: parseInt(s.slice(1, 3), 16), g: parseInt(s.slice(3, 5), 16), b: parseInt(s.slice(5, 7), 16) };
  }
  const m = s.match(/^rgb\(\s*(\d+),\s*(\d+),\s*(\d+)\s*\)$/);
  if (m) return { r: +m[1], g: +m[2], b: +m[3] };
  return null;
}

// Convert {r, g, b} (0-255) to {h: 0-360, s: 0-100, l: 0-100}.
function rgbToHsl(r, g, b) {
  r /= 255; g /= 255; b /= 255;
  const max = Math.max(r, g, b), min = Math.min(r, g, b);
  const l = (max + min) / 2;
  if (max === min) return { h: 0, s: 0, l: l * 100 };
  const d = max - min;
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
  let h;
  switch (max) {
    case r: h = ((g - b) / d + (g < b ? 6 : 0)) / 6; break;
    case g: h = ((b - r) / d + 2) / 6; break;
    default: h = ((r - g) / d + 4) / 6; break;
  }
  return { h: h * 360, s: s * 100, l: l * 100 };
}

// Collect hues (0-360) of all existing labels currently shown in the modal.
function collectExistingLabelHues() {
  const hues = [];
  document.querySelectorAll('.lm-row .cpick-swatch').forEach(swatch => {
    const rgb = parseCSSColor(swatch.style.background);
    if (rgb) hues.push(rgbToHsl(rgb.r, rgb.g, rgb.b).h);
  });
  return hues;
}

// Generate a color whose hue sits in the largest gap between existing hues,
// ensuring it is perceptually distinct from all current labels.
function generateDistinctColor() {
  const hues = collectExistingLabelHues();
  let targetHue;
  if (hues.length === 0) {
    targetHue = Math.floor(Math.random() * 360);
  } else {
    hues.sort((a, b) => a - b);
    let maxGap = 0, maxGapStart = hues[hues.length - 1];
    // Walk the circular hue wheel to find the widest gap.
    for (let i = 0; i < hues.length; i++) {
      const curr = hues[i];
      const next = hues[(i + 1) % hues.length];
      const gap  = i === hues.length - 1 ? (hues[0] + 360 - curr) : (next - curr);
      if (gap > maxGap) { maxGap = gap; maxGapStart = curr; }
    }
    // Midpoint of the largest gap, plus small jitter (±8°) so repeated opens vary slightly.
    targetHue = (maxGapStart + maxGap / 2 + (Math.random() * 16 - 8) + 360) % 360;
  }
  const s = 55 + Math.floor(Math.random() * 30); // 55-85 %
  const l = 45 + Math.floor(Math.random() * 20); // 45-65 %
  return hslToHex(Math.round(targetHue), s, l);
}

// Called every time the modal is (re-)rendered to seed a fresh distinct color.
function initNewLabelColor() {
  syncNewLabelColor(generateDistinctColor());
}

function syncNewLabelColor(color) {
  const swatch = document.getElementById('new-label-swatch');
  if (swatch) swatch.style.background = color;
  const text = document.getElementById('new-label-color-text');
  if (text) text.value = color;
  const picker = document.getElementById('new-label-cpick');
  if (picker && picker.value !== color) picker.value = color;
}

function syncNewLabelColorFromText(value) {
  if (!/^#[0-9a-fA-F]{6}$/.test(value)) return;
  const swatch = document.getElementById('new-label-swatch');
  if (swatch) swatch.style.background = value;
  const picker = document.getElementById('new-label-cpick');
  if (picker) picker.value = value;
}

// ── Existing-label color picker ────────────────────────────────────────────
// Live update swatch + hex code while user drags the picker.
function onLabelColorInput(input) {
  const row = input.closest('.lm-row');
  if (!row) return;
  const color = input.value;
  const swatch = row.querySelector('.cpick-swatch');
  if (swatch) swatch.style.background = color;
  const hex = row.querySelector('.lm-hex');
  if (hex) hex.textContent = color;
  // Keep rename-form color text field in sync
  const labelID = row.dataset.labelId;
  const renameColor = document.getElementById('rename-color-' + labelID);
  if (renameColor) renameColor.value = color;
}

// Auto-save when the color picker closes / a color is committed.
function onLabelColorChange(input) {
  const row = input.closest('.lm-row');
  if (!row) return;
  const boardID = row.dataset.board;
  const labelID = row.dataset.labelId;
  const name    = row.dataset.labelName;
  const color   = input.value;
  fetch(`/ui/boards/${boardID}/labels/${labelID}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': getToken() },
    body: JSON.stringify({ name, color })
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    return r.text();
  }).then(html => {
    document.getElementById('modal-target').innerHTML = html;
    initNewLabelColor();
  }).catch(e => showToast(e.message, 'error'));
}

// Sync rename-form text → swatch + color picker for that row.
function syncRenameColorFromText(input) {
  const value = input.value;
  if (!/^#[0-9a-fA-F]{6}$/.test(value)) return;
  const row = input.closest('.lm-row');
  if (!row) return;
  const swatch = row.querySelector('.cpick-swatch');
  if (swatch) swatch.style.background = value;
  const picker = row.querySelector('.cpick-input');
  if (picker) picker.value = value;
}

// ── Label count badge ─────────────────────────────────────────────────────
// Adjusts the "Labels (N)" header button by delta (+1 or -1) without a reload.
function updateLabelsCountBtn(delta) {
  const btn = document.getElementById('labels-count-btn');
  if (!btn) return;
  const next = (parseInt(btn.dataset.count, 10) || 0) + delta;
  btn.dataset.count = next;
  btn.textContent = `Labels (${next})`;
}

// Reloads card items for all visible lanes to reflect label changes in the
// per-card label picker menus (.card-label-picker-menu).
function refreshAllLaneCards(boardID) {
  document.querySelectorAll('.lane-cards[data-lane]').forEach(function(el) {
    var laneName = el.dataset.lane;
    fetch('/ui/boards/' + boardID + '/lanes/' + encodeURIComponent(laneName) + '/cards', {
      headers: { 'X-Bankan-Token': getToken() }
    }).then(function(r) { return r.text(); }).then(function(html) {
      el.innerHTML = html;
    }).catch(function() {});
  });
}

// Fetches fresh label-picker HTML from the server and updates every
// .label-pick-list in the board (i.e. every add-card inline form).
function refreshLabelPickers(boardID) {
  fetch(`/ui/boards/${boardID}/label-picker`, {
    headers: { 'X-Bankan-Token': getToken() }
  }).then(r => r.text()).then(html => {
    const tmp = document.createElement('div');
    tmp.innerHTML = html;
    const src = tmp.querySelector('.label-pick-list');
    document.querySelectorAll('.label-pick-list').forEach(el => {
      el.innerHTML = src ? src.innerHTML : '';
    });
  }).catch(() => {});
}

// ── Board settings modal ───────────────────────────────────────────────────
function openBoardSettingsModal(boardID) {
  if (!boardID) boardID = document.getElementById('board-view')?.dataset.board;
  if (!boardID) return;
  fetch(`/ui/modals/board-settings/${boardID}`, {
    headers: { 'X-Bankan-Token': getToken() }
  }).then(r => r.text()).then(html => {
    document.getElementById('modal-target').innerHTML = html;
  }).catch(e => showToast(e.message, 'error'));
}

function syncBoardColor(value) {
  const swatch = document.getElementById('bsetting-swatch');
  const text   = document.getElementById('bsetting-color-text');
  const picker = document.getElementById('bsetting-cpick');
  if (swatch) swatch.style.background = value;
  if (text && text.value !== value) text.value = value;
  if (picker && picker.value !== value && /^#[0-9a-fA-F]{6}$/.test(value)) picker.value = value;
}

function submitBoardColor(boardID) {
  const color = document.getElementById('bsetting-color-text')?.value.trim();
  if (!color) return;
  fetch(`/ui/boards/${boardID}/color`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': getToken() },
    body: JSON.stringify({ color })
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    window.location.reload();
  }).catch(e => showToast(e.message, 'error'));
}

// ── Manage labels modal ────────────────────────────────────────────────────
function openManageLabelsModal(boardID) {
  if (!boardID) boardID = document.getElementById('board-view')?.dataset.board;
  if (!boardID) return;
  fetch(`/ui/modals/manage-labels/${boardID}`, {
    headers: { 'X-Bankan-Token': getToken() }
  }).then(r => r.text()).then(html => {
    document.getElementById('modal-target').innerHTML = html;
    initNewLabelColor();
  }).catch(e => showToast(e.message, 'error'));
}

function openRenameLabelForm(labelID) {
  const form = document.getElementById('rename-form-' + labelID);
  if (form) form.style.display = 'flex';
}

function closeRenameLabelForm(labelID) {
  const form = document.getElementById('rename-form-' + labelID);
  if (form) form.style.display = 'none';
}

function submitRenameLabel(boardID, labelID) {
  const name  = document.getElementById('rename-name-' + labelID)?.value.trim();
  const color = document.getElementById('rename-color-' + labelID)?.value.trim();
  if (!name) return;
  fetch(`/ui/boards/${boardID}/labels/${labelID}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': getToken() },
    body: JSON.stringify({ name, color })
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    return r.text();
  }).then(html => {
    document.getElementById('modal-target').innerHTML = html;
    refreshLabelPickers(boardID);
    refreshAllLaneCards(boardID);
    initNewLabelColor();
  }).catch(e => showToast(e.message, 'error'));
}

function openDeleteLabelDialog(boardID, labelID) {
  fetch(`/ui/modals/delete-label/${boardID}/${labelID}`, {
    headers: { 'X-Bankan-Token': getToken() }
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    return r.text();
  }).then(html => {
    document.getElementById('modal-target').innerHTML = html;
  }).catch(e => showToast(e.message, 'error'));
}

function confirmDeleteLabel(boardID, labelID) {
  const chk = document.getElementById('delete-label-archive-chk');
  const archive = chk ? chk.checked : false;
  const url    = archive
    ? `/ui/boards/${boardID}/labels/${labelID}/archive`
    : `/ui/boards/${boardID}/labels/${labelID}`;
  const method = archive ? 'POST' : 'DELETE';
  fetch(url, { method, headers: { 'X-Bankan-Token': getToken() } })
    .then(r => {
      if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
      return r.text();
    })
    .then(html => {
      document.getElementById('modal-target').innerHTML = html;
      if (!archive) updateLabelsCountBtn(-1);
      refreshLabelPickers(boardID);
      refreshAllLaneCards(boardID);
      initNewLabelColor();
    })
    .catch(e => showToast(e.message, 'error'));
}

function submitAddLabelFromModal(event, form) {
  event.preventDefault();
  const boardID = form.dataset.board;
  const name    = form.querySelector('[name=name]').value.trim();
  const color   = form.querySelector('[name=color]').value.trim();
  if (!name || !color) return;
  const fd = new FormData();
  fd.append('name', name);
  fd.append('color', color);
  fetch(`/ui/boards/${boardID}/labels`, {
    method: 'POST',
    headers: { 'X-Bankan-Token': getToken() },
    body: fd
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    return fetch(`/ui/modals/manage-labels/${boardID}`, {
      headers: { 'X-Bankan-Token': getToken() }
    }).then(r2 => r2.text());
  }).then(html => {
    document.getElementById('modal-target').innerHTML = html;
    updateLabelsCountBtn(+1);
    refreshLabelPickers(boardID);
    refreshAllLaneCards(boardID);
    initNewLabelColor();
  }).catch(e => showToast(e.message, 'error'));
}

function submitCreateBoard(event, form) {
  event.preventDefault();
  const name  = form.querySelector('[name=name]').value.trim();
  const token = form.dataset.token || getToken();
  if (!name) return;

  fetch('/api/v1/boards', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': token },
    body: JSON.stringify({ name })
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    return r.json();
  }).then(board => {
    document.getElementById('modal-target').innerHTML = '';
    window.location.href = '/ui/boards/' + board.id;
  }).catch(e => showToast(e.message, 'error'));
}

// ── New board modal type switching ────────────────────────────────────────
function onNewBoardTypeChange(select) {
  const fields = document.getElementById('view-board-fields');
  if (!fields) return;
  if (select.value === 'view') {
    fields.style.display = 'flex';
    const parentSel = document.getElementById('new-view-parent');
    if (parentSel && parentSel.value) {
      onViewParentChange(parentSel);
    }
  } else {
    fields.style.display = 'none';
  }
}

// Fetch labels for the selected parent board and populate the filter label select.
function onViewParentChange(select) {
  const parentID = select.value;
  const target = document.getElementById('new-view-filter-label');
  if (!target) return;
  if (!parentID) {
    target.innerHTML = '<option value="">Select a label…</option>';
    return;
  }
  fetch('/ui/boards/' + parentID + '/labels-fragment')
    .then(r => r.text())
    .then(html => {
      target.innerHTML = '<option value="">Select a label…</option>' + html;
    })
    .catch(e => showToast(e.message, 'error'));
}

// Dispatches to regular board or view board creation based on type selector.
function submitNewBoard(event, form) {
  event.preventDefault();
  const type  = form.querySelector('[name=board_type]').value;
  const token = form.dataset.token || getToken();

  if (type === 'view') {
    const name     = form.querySelector('[name=name]').value.trim();
    const parentID = form.querySelector('[name=parent_id]').value;
    const labelID  = form.querySelector('[name=filter_label_id]').value;
    if (!name || !parentID || !labelID) {
      showToast('Name, parent board and filter label are all required', 'error');
      return;
    }
    fetch('/api/v1/view-boards', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': token },
      body: JSON.stringify({ name, parent_id: parentID, filter_label_id: labelID })
    }).then(r => {
      if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
      return r.json();
    }).then(board => {
      document.getElementById('modal-target').innerHTML = '';
      window.location.href = '/ui/boards/' + board.id;
    }).catch(e => showToast(e.message, 'error'));
  } else {
    const name = form.querySelector('[name=name]').value.trim();
    if (!name) return;
    fetch('/api/v1/boards', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': token },
      body: JSON.stringify({ name })
    }).then(r => {
      if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
      return r.json();
    }).then(board => {
      document.getElementById('modal-target').innerHTML = '';
      window.location.href = '/ui/boards/' + board.id;
    }).catch(e => showToast(e.message, 'error'));
  }
}

// ── Rename lane modal ─────────────────────────────────────────────────────
function openRenameLaneModal(laneName, boardID) {
  closeAllDropdowns();
  const escaped = laneName.replace(/\\/g, '\\\\').replace(/'/g, "\\'");
  const modal = `<div class="modal-backdrop" id="rename-lane-modal" onclick="closeModalOnBackdrop(event)">
    <div class="modal" style="max-width:360px" onclick="event.stopPropagation()">
      <div class="modal-header">
        <span class="modal-title">Rename lane</span>
        <button class="modal-close" onclick="document.getElementById('modal-target').innerHTML=''">✕</button>
      </div>
      <div class="modal-body">
        <input
          type="text"
          id="rename-lane-input"
          data-lane="${laneName.replace(/"/g, '&quot;')}"
          data-board="${boardID}"
          value="${laneName.replace(/"/g, '&quot;')}"
          style="width:100%;box-sizing:border-box"
          onkeydown="if(event.key==='Enter'){submitRenameLane();}else if(event.key==='Escape'){document.getElementById('modal-target').innerHTML='';}"
        />
      </div>
      <div class="form-row" style="padding:0 16px 16px">
        <button class="btn btn-primary btn-sm" onclick="submitRenameLane()">Rename</button>
        <button class="btn btn-ghost btn-sm" onclick="document.getElementById('modal-target').innerHTML=''">Cancel</button>
      </div>
    </div>
  </div>`;
  document.getElementById('modal-target').innerHTML = modal;
  const input = document.getElementById('rename-lane-input');
  if (input) { input.focus(); input.select(); }
}

function submitRenameLane() {
  const input   = document.getElementById('rename-lane-input');
  if (!input) return;
  const laneName = input.dataset.lane;
  const boardID  = input.dataset.board;
  const newName  = input.value.trim();
  if (!newName || newName === laneName) {
    document.getElementById('modal-target').innerHTML = '';
    return;
  }
  fetch(`/api/v1/boards/${boardID}/lanes/${encodeURIComponent(laneName)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': getToken() },
    body: JSON.stringify({ new_name: newName })
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    document.getElementById('modal-target').innerHTML = '';
    reloadBoard(boardID);
  }).catch(e => showToast(e.message, 'error'));
}

// ── Add comment ───────────────────────────────────────────────────────────
function submitComment(event, form) {
  event.preventDefault();
  const boardID = form.dataset.board;
  const cardID  = form.dataset.card;
  const token   = form.dataset.token || getToken();
  const body    = form.querySelector('[name=body]').value.trim();
  const author  = form.querySelector('[name=author]').value.trim();
  if (!body) return;

  fetch(`/ui/boards/${boardID}/cards/${cardID}/comment`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': token },
    body: JSON.stringify({ author, body })
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    return r.text();
  }).then(html => {
    const list = document.getElementById('comments-list');
    if (list) {
      const empty = list.querySelector('.text-dim');
      if (empty) empty.remove();
      list.insertAdjacentHTML('beforeend', html);
    }
    form.reset();
  }).catch(e => showToast(e.message, 'error'));
}

// ── Edit comment ─────────────────────────────────────────────────────────
function openEditComment(commentID) {
  document.getElementById('comment-body-' + commentID).style.display = 'none';
  document.getElementById('comment-edit-' + commentID).style.display = 'block';
  const ta = document.getElementById('comment-edit-textarea-' + commentID);
  if (ta) { ta.focus(); ta.selectionStart = ta.selectionEnd = ta.value.length; }
}

function cancelEditComment(commentID) {
  document.getElementById('comment-body-' + commentID).style.display = '';
  document.getElementById('comment-edit-' + commentID).style.display = 'none';
}

function submitEditComment(commentID) {
  const ta = document.getElementById('comment-edit-textarea-' + commentID);
  const body = ta ? ta.value.trim() : '';
  if (!body) return;
  const form = document.querySelector('form[data-board][data-card]');
  const boardID = form?.dataset.board;
  const cardID  = form?.dataset.card;
  const token   = form?.dataset.token || getToken();
  if (!boardID || !cardID) { showToast('Cannot resolve board/card context', 'error'); return; }

  fetch(`/ui/boards/${boardID}/cards/${cardID}/comments/${commentID}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': token },
    body: JSON.stringify({ body })
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    return r.text();
  }).then(html => {
    const el = document.getElementById('comment-' + commentID);
    if (el) el.outerHTML = html;
  }).catch(e => showToast(e.message, 'error'));
}

// ── Edit card: primary label drag-and-drop ────────────────────────────────
function onEditLabelDragStart(event, el) {
  event.dataTransfer.setData('text/plain', JSON.stringify({
    id: el.dataset.labelId,
    name: el.dataset.labelName,
    color: el.dataset.labelColor
  }));
  event.dataTransfer.effectAllowed = 'copy';
}

function onPrimaryLabelDragOver(event) {
  event.preventDefault();
  event.dataTransfer.dropEffect = 'copy';
  const zone = document.getElementById('primary-label-drop-zone');
  if (zone) zone.classList.add('drag-over');
}

function onPrimaryLabelDragLeave(event) {
  const zone = document.getElementById('primary-label-drop-zone');
  if (zone) zone.classList.remove('drag-over');
}

function onPrimaryLabelDrop(event) {
  event.preventDefault();
  const zone = document.getElementById('primary-label-drop-zone');
  if (zone) zone.classList.remove('drag-over');
  let data;
  try { data = JSON.parse(event.dataTransfer.getData('text/plain')); } catch (_) { return; }
  if (!data || !data.id) return;
  _setPrimaryLabel(data.id, data.name, data.color);
  const checkbox = document.querySelector('input[name=labels][value="' + data.id + '"]');
  if (checkbox) checkbox.checked = true;
}

function clearPrimaryLabel() {
  _setPrimaryLabel('', '', '');
}

function _setPrimaryLabel(id, name, color) {
  const input = document.getElementById('edit-primary-label-input');
  if (input) input.value = id;
  const zone = document.getElementById('primary-label-drop-zone');
  if (!zone) return;
  if (!id) {
    zone.innerHTML = '<span class="text-dim" id="primary-label-placeholder" style="font-size:11px;text-align:center;line-height:1.4">Drop<br>label<br>here</span>';
    return;
  }
  const rgb = _hexToRGB(color);
  const style = 'border-color:' + color + ';color:' + color + ';background:rgba(' + rgb + ',0.12)';
  zone.innerHTML =
    '<span id="primary-label-display" class="card-label card-label-primary" style="' + style + '" title="Primary label">' + _escapeHtml(name) + '</span>' +
    '<button type="button" class="btn-icon" id="clear-primary-label" onclick="clearPrimaryLabel()" title="Clear primary label" style="font-size:11px;padding:2px 5px">✕</button>';
}

function _hexToRGB(hex) {
  if (!hex || hex.length !== 7) return '100,100,100';
  return parseInt(hex.slice(1,3),16) + ',' + parseInt(hex.slice(3,5),16) + ',' + parseInt(hex.slice(5,7),16);
}

function _escapeHtml(str) {
  return str.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

// ── Board card picker: primary label drag-and-drop ───────────────────────
function onBoardLabelDragStart(event, el) {
  event.stopPropagation();
  event.dataTransfer.setData('text/plain', JSON.stringify({
    id: el.dataset.labelId,
    name: el.dataset.labelName,
    color: el.dataset.labelColor
  }));
  event.dataTransfer.effectAllowed = 'copy';
}

function onBoardPrimaryLabelDragOver(event) {
  event.preventDefault();
  event.dataTransfer.dropEffect = 'copy';
  event.currentTarget.classList.add('drag-over');
}

function onBoardPrimaryLabelDragLeave(event) {
  event.currentTarget.classList.remove('drag-over');
}

function onBoardPrimaryLabelDrop(event) {
  event.preventDefault();
  event.stopPropagation();
  const zone = event.currentTarget;
  zone.classList.remove('drag-over');
  let data;
  try { data = JSON.parse(event.dataTransfer.getData('text/plain')); } catch (_) { return; }
  if (!data || !data.id) return;
  const oldPrimary = zone.dataset.primaryLabel || '';
  _updateBoardCardPrimaryLabel(zone.dataset.card, zone.dataset.board, data.id, oldPrimary);
}

function clearBoardPrimaryLabel(btn) {
  const zone = btn.closest('.board-primary-drop-zone');
  if (!zone) return;
  _updateBoardCardPrimaryLabel(zone.dataset.card, zone.dataset.board, '', zone.dataset.primaryLabel || '');
}

function _updateBoardCardPrimaryLabel(cardID, boardID, labelID, oldPrimaryID) {
  const payload = { primary_label: labelID };
  if (oldPrimaryID && oldPrimaryID !== labelID) {
    payload.remove_labels = [oldPrimaryID];
  }
  fetch(`/ui/boards/${boardID}/cards/${cardID}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': getToken() },
    body: JSON.stringify(payload)
  }).then(r => {
    if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
    return r.text();
  }).then(cardHtml => {
    const boardCard = document.getElementById('card-' + cardID);
    if (boardCard) boardCard.outerHTML = cardHtml;
    const newCard = document.getElementById('card-' + cardID);
    if (newCard) {
      const picker = newCard.querySelector('.card-label-picker-menu');
      if (picker) _showBoardPicker(picker);
    }
  }).catch(e => showToast(e.message, 'error'));
}

// ── Board reload ──────────────────────────────────────────────────────────
function reloadBoard(boardID) {
  const savedScrollLeft = document.getElementById('board-view')?.scrollLeft ?? 0;
  fetch(`/ui/boards/${boardID}`)
    .then(r => r.text())
    .then(html => {
      document.open();
      document.write(html);
      document.close();
      const boardEl = document.getElementById('board-view');
      if (boardEl && savedScrollLeft) boardEl.scrollLeft = savedScrollLeft;
    });
}

// ── Lane card count update ────────────────────────────────────────────────
function updateLaneCount(laneName) {
  const cards = document.getElementById('cards-' + laneName);
  const lane  = cards?.closest('.lane');
  const badge = lane?.querySelector('.lane-count');
  if (badge && cards) {
    const count = cards.querySelectorAll('.card').length;
    badge.textContent = count;
  }
}

// ── SortableJS drag-and-drop ──────────────────────────────────────────────
function initSortable() {
  // Cards within lanes.
  document.querySelectorAll('.lane-cards').forEach(function(el) {
    if (el._sortable) { el._sortable.destroy(); }
    el._sortable = Sortable.create(el, {
      group: 'cards',
      animation: 150,
      ghostClass: 'sortable-ghost',
      filter: '.label-draggable',
      preventOnFilter: false,
      onEnd: function(evt) {
        const cardID  = evt.item.dataset.cardId;
        const boardID = evt.item.dataset.board;
        const toLane  = evt.to.dataset.lane;

        if (evt.from === evt.to) {
          // Same lane: reorder within the lane.
          if (evt.oldIndex === evt.newIndex) return;
          const newIndex = evt.newIndex;
          fetch(`/api/v1/boards/${boardID}/cards/${cardID}/reorder`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': getToken() },
            body: JSON.stringify({ new_index: newIndex })
          }).then(r => {
            if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
          }).catch(e => {
            showToast(e.message, 'error');
            // Revert: move card back to original position.
            evt.from.insertBefore(evt.item, evt.from.children[evt.oldIndex] || null);
          });
          return;
        }

        // SortableJS already moved the DOM element; sync the empty-lane
        // placeholder in both affected containers.
        const toEmpty = evt.to.querySelector('.empty-lane');
        if (toEmpty) toEmpty.remove();
        if (evt.from.querySelectorAll('.card').length === 0) {
          evt.from.insertAdjacentHTML('afterbegin', '<div class="empty-lane">No cards yet</div>');
        }

        fetch(`/api/v1/boards/${boardID}/cards/${cardID}/move`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': getToken() },
          body: JSON.stringify({ to_lane: toLane })
        }).then(r => {
          if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
          updateLaneCount(evt.from.dataset.lane);
          updateLaneCount(toLane);
        }).catch(e => {
          showToast(e.message, 'error');
          // Revert: move card back to original position.
          evt.from.insertBefore(evt.item, evt.from.children[evt.oldIndex] || null);
          // Re-sync empty-lane placeholders after revert.
          const fromEmpty = evt.from.querySelector('.empty-lane');
          if (fromEmpty) fromEmpty.remove();
          if (evt.to.querySelectorAll('.card').length === 0) {
            evt.to.insertAdjacentHTML('afterbegin', '<div class="empty-lane">No cards yet</div>');
          }
        });
      }
    });
  });

  // Board tabs.
  const tabNav = document.getElementById('board-tabs');
  if (tabNav) {
    if (tabNav._tabSortable) { tabNav._tabSortable.destroy(); }
    tabNav._tabSortable = Sortable.create(tabNav, {
      animation: 150,
      filter: '.board-tab-archived, .board-tab-hidden',
      draggable: '.board-tab:not(.board-tab-archived):not(.board-tab-hidden)',
      ghostClass: 'sortable-ghost',
      onEnd: function(evt) {
        if (evt.oldIndex === evt.newIndex) return;
        const ids = Array.from(tabNav.querySelectorAll('.board-tab:not(.board-tab-archived):not(.board-tab-hidden)[data-board-id]'))
          .map(function(el) { return el.dataset.boardId; });
        fetch('/api/v1/boards/reorder', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': getToken() },
          body: JSON.stringify({ ids: ids })
        }).then(function(r) {
          if (!r.ok) return r.json().then(function(d) { throw new Error(d.error || 'Failed'); });
        }).catch(function(e) {
          showToast(e.message, 'error');
        });
      }
    });
  }

  // Lanes within the board view.
  const boardEl = document.getElementById('board-view');
  if (boardEl) {
    if (boardEl._laneSortable) { boardEl._laneSortable.destroy(); }
    boardEl._laneSortable = Sortable.create(boardEl, {
      animation: 150,
      handle: '.lane-header',
      draggable: '.lane',
      filter: '.add-lane-col',
      ghostClass: 'sortable-ghost',
      onEnd: function(evt) {
        if (evt.oldIndex === evt.newIndex) return;
        const boardID = boardEl.dataset.board;
        if (!boardID) return;
        const names = Array.from(boardEl.querySelectorAll('.lane[data-lane]'))
          .map(el => el.dataset.lane);
        fetch(`/api/v1/boards/${boardID}/lanes/reorder`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': getToken() },
          body: JSON.stringify({ names })
        }).then(r => {
          if (!r.ok) return r.json().then(d => { throw new Error(d.error || 'Failed'); });
        }).catch(e => {
          showToast(e.message, 'error');
          reloadBoard(boardID);
        });
      }
    });
  }
}

// ── Toast notifications ───────────────────────────────────────────────────
function showToast(message, type = 'success') {
  const area = document.getElementById('toast-area');
  if (!area) return;
  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  toast.textContent = message;
  area.appendChild(toast);
  setTimeout(() => toast.remove(), 4000);
}

// ── Hovered card tracking (for keyboard shortcuts) ────────────────────────
var _hoveredCard = null; // {cardId, boardId} | null

document.addEventListener('mouseover', function(e) {
  var card = e.target.closest('.card:not(.card-archived)');
  if (card && card.dataset.cardId && card.dataset.board) {
    _hoveredCard = { cardId: card.dataset.cardId, boardId: card.dataset.board };
  }
});

document.addEventListener('mouseout', function(e) {
  if (!_hoveredCard) return;
  var card = e.target.closest('.card:not(.card-archived)');
  if (!card) return;
  if (!card.contains(e.relatedTarget)) {
    _hoveredCard = null;
  }
});

// ── Keyboard shortcuts ────────────────────────────────────────────────────
document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') {
    closeModal();
    closeAllDropdowns();
    closeAllCardLabelPickers();
    closeAddLaneForm();
    closeAllAddCardForms();
  }

  if ((e.key === 'e' || e.key === 'm') && !e.ctrlKey && !e.altKey && !e.metaKey) {
    var active = document.activeElement;
    var tag = active ? active.tagName.toLowerCase() : '';
    if (tag === 'input' || tag === 'textarea' || tag === 'select' || (active && active.isContentEditable)) return;

    var cardId, boardId;
    var cardModal = document.getElementById('card-modal');
    if (cardModal) {
      // Only act when the card is editable (Edit/Move buttons are present).
      if (!cardModal.querySelector('button[onclick*="openEditCardModal"]')) return;
      cardId = cardModal.dataset.card;
      boardId = cardModal.dataset.board;
    } else if (_hoveredCard) {
      cardId = _hoveredCard.cardId;
      boardId = _hoveredCard.boardId;
    }

    if (!cardId || !boardId) return;
    e.preventDefault();
    if (e.key === 'e') {
      openEditCardModal(cardId, boardId);
    } else {
      openMoveCardModal(cardId, boardId);
    }
  }
});

// ── HTMX after-settle: re-init sortable ──────────────────────────────────
document.addEventListener('htmx:afterSettle', function() {
  initSortable();
  initArchivedBoardsVisibility();
  initLaneExpandState();
});

// ── Card permalink: auto-open on page load ────────────────────────────────

// Scrolls to a comment inside the open card modal and briefly highlights it.
function _scrollToComment(commentId) {
  var el = document.getElementById('comment-' + commentId);
  if (!el) return;
  el.scrollIntoView({ behavior: 'smooth', block: 'center' });
  el.classList.add('comment-highlight');
  setTimeout(function() { el.classList.remove('comment-highlight'); }, 2000);
}

// Opens the card detail modal from a URL that already contains ?card=.
// Does NOT push to history (the URL is already correct).
function _autoOpenCardFromUrl() {
  var params = new URLSearchParams(location.search);
  var cardId = params.get('card');
  if (!cardId) return;
  var boardId = _extractBoardIdFromPath();
  if (!boardId) return;
  fetch(`/ui/modals/card/${cardId}/boards/${boardId}`, {
    headers: { 'X-Bankan-Token': getToken() }
  }).then(function(r) {
    if (!r.ok) return r.json().then(function(d) { throw new Error(d.error || 'Failed'); });
    return r.text();
  }).then(function(html) {
    document.getElementById('modal-target').innerHTML = html;
    var commentId = new URLSearchParams(location.search).get('comment');
    if (commentId) {
      setTimeout(function() { _scrollToComment(commentId); }, 120);
    }
  }).catch(function(e) { showToast(e.message, 'error'); });
}

// ── Lane expand toggle ────────────────────────────────────────────────────
var LANE_EXPAND_KEY_PREFIX = 'bankan.lane-expanded.';

function _syncBoardViewScroll() {
  var boardView = document.getElementById('board-view');
  if (!boardView) return;
  var hasExpanded = !!boardView.querySelector('.lane.lane-expanded');
  boardView.style.overflowY = hasExpanded ? 'auto' : '';
}

function toggleLaneExpandByBtn(btn) {
  var lane = btn.closest('.lane');
  if (!lane) return;
  var laneId = lane.dataset.laneId;
  var isExpanded = lane.classList.toggle('lane-expanded');
  btn.textContent = isExpanded ? '↕ Collapse' : '↕ Expand';
  var key = LANE_EXPAND_KEY_PREFIX + laneId;
  if (isExpanded) {
    sessionStorage.setItem(key, '1');
  } else {
    sessionStorage.removeItem(key);
  }
  _syncBoardViewScroll();
  closeAllDropdowns();
}

function initLaneExpandState() {
  document.querySelectorAll('.lane[data-lane-id]').forEach(function(lane) {
    var key = LANE_EXPAND_KEY_PREFIX + lane.dataset.laneId;
    if (sessionStorage.getItem(key)) {
      lane.classList.add('lane-expanded');
      var btn = lane.querySelector('.lane-expand-btn');
      if (btn) btn.textContent = '↕ Collapse';
    }
  });
  _syncBoardViewScroll();
}

// ── Init ──────────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', function() {
  initSortable();
  initArchivedBoardsVisibility();
  initLaneExpandState();
  _autoOpenCardFromUrl();
});

// ── Browser back/forward: sync modal with URL ─────────────────────────────
window.addEventListener('popstate', function() {
  var params = new URLSearchParams(location.search);
  var cardId = params.get('card');
  if (!cardId) {
    // URL no longer has a card — close any open modal without touching history.
    document.getElementById('modal-target').innerHTML = '';
    return;
  }
  var boardId = _extractBoardIdFromPath();
  if (!boardId) return;
  fetch(`/ui/modals/card/${cardId}/boards/${boardId}`, {
    headers: { 'X-Bankan-Token': getToken() }
  }).then(function(r) {
    if (!r.ok) return;
    return r.text();
  }).then(function(html) {
    if (!html) return;
    document.getElementById('modal-target').innerHTML = html;
    var commentId = new URLSearchParams(location.search).get('comment');
    if (commentId) {
      setTimeout(function() { _scrollToComment(commentId); }, 120);
    }
  });
});

// ── Board visibility (hidden + archived view boards) ─────────────────────

var ARCHIVED_LS_KEY = 'bankan.visible-archived-boards';

function _getVisibleArchivedIDs() {
  try {
    var val = localStorage.getItem(ARCHIVED_LS_KEY);
    return val ? JSON.parse(val) : [];
  } catch (_) { return []; }
}

function _setVisibleArchivedIDs(ids) {
  try {
    localStorage.setItem(ARCHIVED_LS_KEY, JSON.stringify(ids));
  } catch (_) {}
}

// Called on page load. Shows any archived tab whose ID is in localStorage.
// Also: if the currently active tab is itself an archived board (page was
// loaded directly on an archived board), show it and add it to localStorage
// so it persists across refreshes.
function initArchivedBoardsVisibility() {
  var visible = _getVisibleArchivedIDs();
  var changed = false;
  document.querySelectorAll('.board-tab-archived[data-archived-id]').forEach(function(tab) {
    var id = tab.dataset.archivedId;
    if (tab.classList.contains('active')) {
      tab.style.display = '';
      if (visible.indexOf(id) === -1) { visible.push(id); changed = true; }
    } else if (visible.indexOf(id) !== -1) {
      tab.style.display = '';
    }
  });
  if (changed) { _setVisibleArchivedIDs(visible); }
  _syncArchivedDropdownItems();
}

// Hides dropdown items for archived boards that are already visible in the tab
// bar, and shows the section title only when at least one item is visible.
function _syncArchivedDropdownItems() {
  var visible = _getVisibleArchivedIDs();
  var activeTab = document.querySelector('.board-tab-archived.active');
  if (activeTab && activeTab.dataset.archivedId) {
    var aid = activeTab.dataset.archivedId;
    if (visible.indexOf(aid) === -1) { visible.push(aid); }
  }
  var anyVisible = false;
  document.querySelectorAll('[data-archived-item-id]').forEach(function(item) {
    if (visible.indexOf(item.dataset.archivedItemId) !== -1) {
      item.style.display = 'none';
    } else {
      item.style.display = '';
      anyVisible = true;
    }
  });
  var title = document.getElementById('archived-views-section-title');
  if (title) { title.style.display = anyVisible ? '' : 'none'; }
}

// Toggles the unified boards overflow dropdown panel (hidden boards + archived views).
function toggleBoardsOverflowPanel() {
  var panel = document.getElementById('boards-overflow-panel');
  if (!panel) return;
  panel.style.display = panel.style.display === 'none' ? '' : 'none';
}

// Shows an archived tab in the tab bar and persists the choice to localStorage.
function showArchivedTab(event, id) {
  if (event) event.preventDefault();
  toggleBoardsOverflowPanel();
  var tab = document.querySelector('.board-tab-archived[data-archived-id="' + id + '"]');
  if (tab) tab.style.display = '';
  var ids = _getVisibleArchivedIDs();
  if (ids.indexOf(id) === -1) { ids.push(id); }
  _setVisibleArchivedIDs(ids);
  _syncArchivedDropdownItems();
  // Navigate to the board.
  if (event) {
    var href = event.currentTarget ? event.currentTarget.getAttribute('href') : null;
    if (href) { window.location.href = href; }
  }
}

// Hides an archived tab from the tab bar and removes it from localStorage.
// If this was the currently active board, navigates to the first visible board.
function hideArchivedTab(event, id) {
  if (event) { event.stopPropagation(); event.preventDefault(); }
  var tab = document.querySelector('.board-tab-archived[data-archived-id="' + id + '"]');
  var wasActive = tab && tab.classList.contains('active');
  if (tab) tab.style.display = 'none';
  var ids = _getVisibleArchivedIDs().filter(function(v) { return v !== id; });
  _setVisibleArchivedIDs(ids);
  _syncArchivedDropdownItems();
  if (wasActive) {
    var firstActive = document.querySelector('.board-tab:not(.board-tab-archived):not(.board-tab-hidden)');
    if (firstActive) {
      window.location.href = firstActive.getAttribute('href');
    } else {
      window.location.href = '/';
    }
  }
}

// Hides a regular or view board from the tab bar (server-side persistence).
// If the board was the currently active one, the server returns a navigate_to target.
function hideBoard(event, id) {
  if (event) { event.stopPropagation(); event.preventDefault(); }
  fetch('/ui/boards/' + id + '/hide', {
    method: 'POST',
    headers: { 'X-Bankan-Token': getToken(), 'HX-Current-URL': location.href }
  }).then(function(r) {
    var tab = document.querySelector('[data-board-id="' + id + '"]');
    if (tab) tab.style.display = 'none';
    if (r.status === 200) {
      return r.json().then(function(data) {
        if (data.navigate_to) {
          window.location.href = '/ui/boards/' + data.navigate_to;
        } else {
          window.location.href = '/';
        }
      });
    }
  }).catch(function(e) {
    showToast(e.message, 'error');
  });
}

// Restores a hidden board to the tab bar (server-side persistence).
// The tab element is already in the DOM at the correct position (display:none);
// this function simply makes it visible again.
function showBoardFromDropdown(event, id) {
  if (event) event.preventDefault();
  toggleBoardsOverflowPanel();
  fetch('/ui/boards/' + id + '/show', {
    method: 'POST',
    headers: { 'X-Bankan-Token': getToken() }
  }).then(function(r) {
    if (!r.ok) return;
    var tab = document.querySelector('[data-hidden-id="' + id + '"]');
    if (tab) {
      tab.style.display = '';
      tab.classList.remove('board-tab-hidden');
      // Promote from hidden-tab to regular tab so SortableJS can include it.
      tab.removeAttribute('data-hidden-id');
      tab.dataset.boardId = id;
    }
    // On the empty state page (no board content rendered), navigate to the restored board.
    if (!document.getElementById('board-view')) {
      window.location.href = '/ui/boards/' + id;
    }
  }).catch(function(e) {
    showToast(e.message, 'error');
  });
}

// Opens the archive-view-board confirmation dialog by fetching it from the server.
function openArchiveViewBoardDialog(boardID, boardName, filterLabelID) {
  fetch('/ui/modals/archive-view-board/' + boardID, {
    headers: { 'X-Bankan-Token': getToken() }
  }).then(function(r) {
    if (!r.ok) return r.json().then(function(d) { throw new Error(d.error || 'Failed'); });
    return r.text();
  }).then(function(html) {
    document.getElementById('modal-target').innerHTML = html;
  }).catch(function(e) {
    showToast(e.message, 'error');
  });
}

// Sends the archive request for a view board.
function confirmArchiveViewBoard(boardID) {
  var archiveLabel = false;
  var chk = document.getElementById('archive-vb-label-chk');
  if (chk) archiveLabel = chk.checked;
  fetch('/api/v1/boards/' + boardID + '/archive', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Bankan-Token': getToken() },
    body: JSON.stringify({ archive_label: archiveLabel })
  }).then(function(r) {
    if (!r.ok) return r.json().then(function(d) { throw new Error(d.error || 'Failed'); });
    document.getElementById('modal-target').innerHTML = '';
    // Navigate away from the now-archived board to the first available board.
    window.location.href = '/';
  }).catch(function(e) {
    showToast(e.message, 'error');
  });
}

// Opens the static markdown syntax cheatsheet modal.
// Saves the current #modal-target content and restores it on close, so that
// opening hints from inside the edit-card modal returns to that modal rather
// than to the board view.
function openMarkdownHintsModal() {
  var target = document.getElementById('modal-target');
  var savedHTML = target.innerHTML;
  fetch('/ui/markdown-hints').then(function(r) {
    if (!r.ok) return;
    return r.text();
  }).then(function(html) {
    if (!html) return;
    target.innerHTML = html;
    var modal = document.getElementById('md-hints-modal');
    if (!modal) return;
    function restorePrev() { target.innerHTML = savedHTML; }
    // Patch close button (top ✕)
    var closeBtn = modal.querySelector('.modal-close');
    if (closeBtn) closeBtn.onclick = restorePrev;
    // Patch bottom Close button
    modal.querySelectorAll('button').forEach(function(btn) {
      if (btn.textContent.trim() === 'Close') btn.onclick = restorePrev;
    });
    // Backdrop click
    modal.onclick = function(e) { if (e.target === modal) restorePrev(); };
  });
}
