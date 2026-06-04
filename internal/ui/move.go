package ui

// ---- Drag & drop: move/reorder a request, including across folders ----
//
// This is the DATA-LAYER for "reorder a request by drag & drop". Dev B wires
// the Fyne Draggable + drop-indicator UI to the two exported entry points here:
//
//	func (w *Window) moveRequest(src int, destFolderID string, beforeReqIdx int)
//	func (w *Window) dropTarget(srcReqIdx, visibleRow int, below bool) (destFolderID string, beforeReqIdx int)
//
// moveRequest mutates the model (reparents + reorders the flat Requests slice)
// and remaps ALL index-keyed UI state (openTabs / each rt.idx / selectedID) so
// no tab is opened or closed, pointers are preserved, and the same request stays
// selected. dropTarget is a PURE helper that turns "the pointer is over visible
// row R, in its lower/upper half" into the (destFolderID, beforeReqIdx) pair
// moveRequest expects — so the drop geometry can be unit-tested without any drag
// events.
//
// Display-vs-flat order (why a within-folder reorder is enough): the sidebar is
// grouped by folder (see Window.sidebarRows) — for each folder it walks the flat
// Requests slice in order and emits the requests whose FolderID matches, then the
// top-level ones last. So a request's DISPLAYED position is decided only by its
// flat order RELATIVE TO THE OTHER REQUESTS IN THE SAME FOLDER. moveRequest still
// performs a fully general, stable slice move (any element to any position), so
// the flat order is always sensible and the permutation remap stays correct
// regardless of where the element lands.

// noMoveTarget is the sentinel beforeReqIdx meaning "place at the end" (of the
// collection / the destination folder group) rather than before a specific
// request. Dev B passes it (or any out-of-range index) for an end-of-list drop.
const noMoveTarget = -1

// moveRequest moves the request at flat index src so it lands in folder
// destFolderID ("" = top-level) and, within the flat slice, immediately BEFORE
// the request currently at flat index beforeReqIdx. Pass beforeReqIdx ==
// noMoveTarget (-1, or any out-of-range index) to place src at the END of the
// slice.
//
// It is a MOVE, never a copy: the slice length is unchanged and every request
// keeps its content; nothing is lost or duplicated. After reordering it remaps
// every piece of index-keyed UI state so the move is invisible to open tabs and
// the selection (see the remap block).
//
// Guards (all no-ops, never a panic / corruption): src out of range; a non-""
// destFolderID that names no existing folder; a move that wouldn't change the
// slice order at all (already in place, or src == beforeReqIdx). Even a no-op
// move still applies the FolderID change (so a same-position cross-folder drop
// reparents) and marks dirty + refreshes if anything changed.
func (w *Window) moveRequest(src int, destFolderID string, beforeReqIdx int) {
	if src < 0 || src >= len(w.coll.Requests) {
		return
	}
	// Reject an unknown destination folder (top-level "" is always valid).
	if destFolderID != "" {
		if _, ok := w.folderByID(destFolderID); !ok {
			return
		}
	}

	// Reparent first; the destination is recorded even if the slice order ends up
	// unchanged (e.g. dropping a request onto a new folder at the same flat spot).
	folderChanged := w.coll.Requests[src].FolderID != destFolderID
	w.coll.Requests[src].FolderID = destFolderID

	// Compute the destination slot in the OLD index space: src must end up
	// immediately before the element currently at beforeReqIdx. A sentinel /
	// out-of-range anchor means "append at the end". An anchor that IS src means
	// "before yourself" — i.e. stay put — so we target src's own slot (moveSlice
	// treats that as a no-op) rather than appending to the end.
	dst := len(w.coll.Requests) // default: append at the end
	switch {
	case beforeReqIdx == src:
		dst = src // before yourself == stay in place
	case beforeReqIdx >= 0 && beforeReqIdx < len(w.coll.Requests):
		dst = beforeReqIdx
	}

	// perm[old] = new maps each element's old flat index to its new one. A nil
	// perm means the slice order is unchanged (identity); we still may have
	// reparented above, so we account for that when deciding to refresh.
	perm := moveSlice(w.coll.Requests, src, dst)

	if perm == nil {
		// Order didn't change. If the folder changed it's still a real edit.
		if folderChanged {
			w.markDirty()
			w.refreshSidebar()
			if w.selectedID == src {
				// Selection stays at the same flat index; just keep the row's
				// accent in sync now that it sits under a different folder.
				w.selectByReqIdx(src)
			}
		}
		return
	}

	// --- Remap the index-keyed UI state through the permutation -------------
	//
	// openTabs and selectedID are keyed by FLAT request index, and a move is a
	// general permutation: any subset of indices between src and dst shifts by
	// one (and src jumps across them), so — unlike delete/duplicate's single
	// +1/-1 shift — we must remap EVERY key through perm. We rebuild openTabs into
	// a FRESH map (an in-place rekey could clobber a not-yet-moved entry) keeping
	// the SAME *requestTab pointers, sync each tab's own rt.idx, and move
	// selectedID with its request so selection follows the moved (or any other)
	// request to its new flat index. No tab is opened or closed.
	remapped := make(map[int]*requestTab, len(w.openTabs))
	for oldIdx, rt := range w.openTabs {
		newIdx := perm[oldIdx]
		rt.idx = newIdx
		remapped[newIdx] = rt
	}
	w.openTabs = remapped

	if w.selectedID >= 0 && w.selectedID < len(perm) {
		w.selectedID = perm[w.selectedID]
	}

	w.markDirty()
	w.refreshSidebar()
	// Keep the List's visible-row selection aligned with the (possibly moved)
	// selected request so its cyan accent stays put after the rows are regrouped.
	if w.selectedID >= 0 {
		w.selectByReqIdx(w.selectedID)
	}
	w.updateStatusBar()
}

// moveSlice performs a STABLE in-place move of the element at index src to the
// position immediately before the element that is currently at index dst (in the
// OLD index space). dst == len(s) means "append at the end". It returns the
// old-index → new-index permutation (perm[old] = new) describing exactly how the
// slice was reordered, or NIL when the move is a no-op (order unchanged), so the
// caller can skip the remap.
//
// The move is stable: every element other than src keeps its relative order, and
// src is reinserted at the requested slot. This is the general permutation the
// openTabs/selectedID remap is built from.
func moveSlice[T any](s []T, src, dst int) []int {
	n := len(s)
	if src < 0 || src >= n {
		return nil
	}
	// Clamp dst into [0, n]; dst == n means append at the end.
	if dst < 0 {
		dst = 0
	}
	if dst > n {
		dst = n
	}

	// Translate "before the element currently at dst" into a destination index in
	// the slice AFTER src is removed. Removing src shifts every element after it
	// down by one, so an anchor that sat after src lands one slot earlier.
	insertAt := dst
	if dst > src {
		insertAt = dst - 1
	}
	if insertAt == src {
		// Re-inserting at its own spot changes nothing.
		return nil
	}

	// Build perm by simulating the remove+insert on an index list, so perm exactly
	// mirrors the element move applied below (single source of truth for "where
	// did old index i go").
	order := make([]int, 0, n) // order[newPos] = oldIndex
	for i := 0; i < n; i++ {
		if i == src {
			continue
		}
		order = append(order, i)
	}
	// Insert src at insertAt within the removed-element list.
	order = append(order, 0)
	copy(order[insertAt+1:], order[insertAt:])
	order[insertAt] = src

	perm := make([]int, n) // perm[oldIndex] = newPos
	for newPos, oldIdx := range order {
		perm[oldIdx] = newPos
	}

	// Apply the same reordering to the real slice using the perm (out-of-place to
	// keep it simple and obviously correct, then copy back).
	moved := make([]T, n)
	for oldIdx := range s {
		moved[perm[oldIdx]] = s[oldIdx]
	}
	copy(s, moved)

	return perm
}

// dropTarget is a PURE helper (no Fyne, no drag events) that maps a drop gesture
// to the (destFolderID, beforeReqIdx) pair moveRequest expects. Given the dragged
// request's flat index srcReqIdx, the VISIBLE row the pointer is over (visibleRow,
// an index into w.rows / sidebarRows()), and whether the pointer is in the LOWER
// half of that row (below == true), it returns where the request should land.
//
// Rules (matching the spec):
//   - Over a REQUEST row → the SAME folder as that row. Anchor before that row's
//     request, or before the NEXT request in display order when below (so a
//     lower-half drop lands after it). Below the row's folder's last request →
//     end of that folder group (beforeReqIdx = noMoveTarget) but still in that
//     folder.
//   - Over a FOLDER HEADER row → into that folder, at the TOP (before its first
//     request). An empty / collapsed folder just sets the folder (anchor =
//     noMoveTarget).
//   - Below the last row, or an out-of-range visibleRow → top-level, at the END
//     (destFolderID "", beforeReqIdx = noMoveTarget).
//
// srcReqIdx is accepted so callers have it to hand (and for future "don't anchor
// on yourself" logic); the anchor returned never depends on src — moveRequest
// already treats beforeReqIdx == src as "append", so dropping a request just
// after itself is a safe no-op there.
func (w *Window) dropTarget(srcReqIdx, visibleRow int, below bool) (destFolderID string, beforeReqIdx int) {
	rows := w.rows

	// Below the list (or a bad row) → top-level, at the very end.
	if visibleRow < 0 || visibleRow >= len(rows) {
		return "", noMoveTarget
	}

	row := rows[visibleRow]

	if row.IsFolder {
		// Drop onto a folder header → that folder, before its first request (top of
		// the group). If the folder has no visible requests, just set the folder.
		first := w.firstReqRowAfter(visibleRow, row.FolderID)
		return row.FolderID, first
	}

	// Drop over a request row → same folder as that row.
	destFolderID = row.FolderID
	if !below {
		// Upper half: land directly before this request.
		return destFolderID, row.ReqIdx
	}
	// Lower half: land before the NEXT request in the same folder group; if this is
	// the last request of its group, append to the group (end sentinel).
	beforeReqIdx = w.nextReqInGroup(visibleRow, destFolderID)
	return destFolderID, beforeReqIdx
}

// firstReqRowAfter returns the flat ReqIdx of the first request row belonging to
// folderID that appears in the visible rows after headerRow (the folder header's
// visible index), or noMoveTarget when the folder shows no request rows (empty or
// collapsed). Used to anchor a drop onto a folder header at the TOP of the group.
func (w *Window) firstReqRowAfter(headerRow int, folderID string) int {
	for i := headerRow + 1; i < len(w.rows); i++ {
		r := w.rows[i]
		if r.IsFolder {
			break // reached the next folder header; this folder has no rows shown
		}
		if r.FolderID == folderID {
			return r.ReqIdx
		}
	}
	return noMoveTarget
}

// nextReqInGroup returns the flat ReqIdx of the request row that follows the one
// at visible index fromRow within the SAME folder group (folderID), scanning the
// visible rows downward. It returns noMoveTarget when fromRow is the last request
// of its group, meaning "append to the end of this folder group". A folder header
// ends the group scan (the next group's requests must not be used as an anchor).
func (w *Window) nextReqInGroup(fromRow int, folderID string) int {
	for i := fromRow + 1; i < len(w.rows); i++ {
		r := w.rows[i]
		if r.IsFolder {
			break // next group begins; nothing after us in this group
		}
		if r.FolderID == folderID {
			return r.ReqIdx
		}
		// A request row from a different folder (shouldn't interleave within a
		// group, but stay defensive) also ends our group scan.
		break
	}
	return noMoveTarget
}
