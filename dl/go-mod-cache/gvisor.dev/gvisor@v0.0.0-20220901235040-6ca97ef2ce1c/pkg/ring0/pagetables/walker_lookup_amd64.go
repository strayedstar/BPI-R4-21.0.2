//go:build amd64
// +build amd64

package pagetables

// iterateRangeCanonical walks a canonical range.
//
//go:nosplit
func (w *lookupWalker) iterateRangeCanonical(start, end uintptr) bool {
	for pgdIndex := uint16((start & pgdMask) >> pgdShift); start < end && pgdIndex < entriesPerPage; pgdIndex++ {
		var (
			pgdEntry   = &w.pageTables.root[pgdIndex]
			pudEntries *PTEs
		)
		if !pgdEntry.Valid() {
			if !w.visitor.requiresAlloc() {

				start = lookupnext(start, pgdSize)
				continue
			}

			pudEntries = w.pageTables.Allocator.NewPTEs()
			pgdEntry.setPageTable(w.pageTables, pudEntries)
		} else {
			pudEntries = w.pageTables.Allocator.LookupPTEs(pgdEntry.Address())
		}

		clearPUDEntries := uint16(0)

		for pudIndex := uint16((start & pudMask) >> pudShift); start < end && pudIndex < entriesPerPage; pudIndex++ {
			var (
				pudEntry   = &pudEntries[pudIndex]
				pmdEntries *PTEs
			)
			if !pudEntry.Valid() {
				if !w.visitor.requiresAlloc() {

					clearPUDEntries++
					start = lookupnext(start, pudSize)
					continue
				}

				if start&(pudSize-1) == 0 && end-start >= pudSize {
					pudEntry.SetSuper()
					if !w.visitor.visit(uintptr(start&^(pudSize-1)), pudEntry, pudSize-1) {
						return false
					}
					if pudEntry.Valid() {
						start = lookupnext(start, pudSize)
						continue
					}
				}

				pmdEntries = w.pageTables.Allocator.NewPTEs()
				pudEntry.setPageTable(w.pageTables, pmdEntries)

			} else if pudEntry.IsSuper() {

				if w.visitor.requiresSplit() && (start&(pudSize-1) != 0 || end < lookupnext(start, pudSize)) {

					pmdEntries = w.pageTables.Allocator.NewPTEs()
					for index := uint16(0); index < entriesPerPage; index++ {
						pmdEntries[index].SetSuper()
						pmdEntries[index].Set(
							pudEntry.Address()+(pmdSize*uintptr(index)),
							pudEntry.Opts())
					}
					pudEntry.setPageTable(w.pageTables, pmdEntries)
				} else {

					if !w.visitor.visit(uintptr(start&^(pudSize-1)), pudEntry, pudSize-1) {
						return false
					}

					if !pudEntry.Valid() {
						clearPUDEntries++
					}

					start = lookupnext(start, pudSize)
					continue
				}
			} else {
				pmdEntries = w.pageTables.Allocator.LookupPTEs(pudEntry.Address())
			}

			clearPMDEntries := uint16(0)

			for pmdIndex := uint16((start & pmdMask) >> pmdShift); start < end && pmdIndex < entriesPerPage; pmdIndex++ {
				var (
					pmdEntry   = &pmdEntries[pmdIndex]
					pteEntries *PTEs
				)
				if !pmdEntry.Valid() {
					if !w.visitor.requiresAlloc() {

						clearPMDEntries++
						start = lookupnext(start, pmdSize)
						continue
					}

					if start&(pmdSize-1) == 0 && end-start >= pmdSize {
						pmdEntry.SetSuper()
						if !w.visitor.visit(uintptr(start&^(pmdSize-1)), pmdEntry, pmdSize-1) {
							return false
						}
						if pmdEntry.Valid() {
							start = lookupnext(start, pmdSize)
							continue
						}
					}

					pteEntries = w.pageTables.Allocator.NewPTEs()
					pmdEntry.setPageTable(w.pageTables, pteEntries)

				} else if pmdEntry.IsSuper() {

					if w.visitor.requiresSplit() && (start&(pmdSize-1) != 0 || end < lookupnext(start, pmdSize)) {

						pteEntries = w.pageTables.Allocator.NewPTEs()
						for index := uint16(0); index < entriesPerPage; index++ {
							pteEntries[index].Set(
								pmdEntry.Address()+(pteSize*uintptr(index)),
								pmdEntry.Opts())
						}
						pmdEntry.setPageTable(w.pageTables, pteEntries)
					} else {

						if !w.visitor.visit(uintptr(start&^(pmdSize-1)), pmdEntry, pmdSize-1) {
							return false
						}

						if !pmdEntry.Valid() {
							clearPMDEntries++
						}

						start = lookupnext(start, pmdSize)
						continue
					}
				} else {
					pteEntries = w.pageTables.Allocator.LookupPTEs(pmdEntry.Address())
				}

				clearPTEEntries := uint16(0)

				for pteIndex := uint16((start & pteMask) >> pteShift); start < end && pteIndex < entriesPerPage; pteIndex++ {
					var (
						pteEntry = &pteEntries[pteIndex]
					)
					if !pteEntry.Valid() && !w.visitor.requiresAlloc() {
						clearPTEEntries++
						start += pteSize
						continue
					}

					if !w.visitor.visit(uintptr(start&^(pteSize-1)), pteEntry, pteSize-1) {
						return false
					}
					if !pteEntry.Valid() && !w.visitor.requiresAlloc() {
						clearPTEEntries++
					}

					start += pteSize
					continue
				}

				if clearPTEEntries == entriesPerPage {
					pmdEntry.Clear()
					w.pageTables.Allocator.FreePTEs(pteEntries)
					clearPMDEntries++
				}
			}

			if clearPMDEntries == entriesPerPage {
				pudEntry.Clear()
				w.pageTables.Allocator.FreePTEs(pmdEntries)
				clearPUDEntries++
			}
		}

		if clearPUDEntries == entriesPerPage {
			pgdEntry.Clear()
			w.pageTables.Allocator.FreePTEs(pudEntries)
		}
	}
	return true
}

// Walker walks page tables.
type lookupWalker struct {
	// pageTables are the tables to walk.
	pageTables *PageTables

	// Visitor is the set of arguments.
	visitor lookupVisitor
}

// iterateRange iterates over all appropriate levels of page tables for the given range.
//
// If requiresAlloc is true, then Set _must_ be called on all given PTEs. The
// exception is super pages. If a valid super page (huge or jumbo) cannot be
// installed, then the walk will continue to individual entries.
//
// This algorithm will attempt to maximize the use of super/sect pages whenever
// possible. Whether a super page is provided will be clear through the range
// provided in the callback.
//
// Note that if requiresAlloc is true, then no gaps will be present. However,
// if alloc is not set, then the iteration will likely be full of gaps.
//
// Note that this function should generally be avoided in favor of Map, Unmap,
// etc. when not necessary.
//
// Precondition: start must be page-aligned.
// Precondition: start must be less than end.
// Precondition: If requiresAlloc is true, then start and end should not span
// non-canonical ranges. If they do, a panic will result.
//
//go:nosplit
func (w *lookupWalker) iterateRange(start, end uintptr) {
	if start%pteSize != 0 {
		panic("unaligned start")
	}
	if end < start {
		panic("start > end")
	}
	if start < lowerTop {
		if end <= lowerTop {
			w.iterateRangeCanonical(start, end)
		} else if end > lowerTop && end <= upperBottom {
			if w.visitor.requiresAlloc() {
				panic("alloc spans non-canonical range")
			}
			w.iterateRangeCanonical(start, lowerTop)
		} else {
			if w.visitor.requiresAlloc() {
				panic("alloc spans non-canonical range")
			}
			if !w.iterateRangeCanonical(start, lowerTop) {
				return
			}
			w.iterateRangeCanonical(upperBottom, end)
		}
	} else if start < upperBottom {
		if end <= upperBottom {
			if w.visitor.requiresAlloc() {
				panic("alloc spans non-canonical range")
			}
		} else {
			if w.visitor.requiresAlloc() {
				panic("alloc spans non-canonical range")
			}
			w.iterateRangeCanonical(upperBottom, end)
		}
	} else {
		w.iterateRangeCanonical(start, end)
	}
}

// next returns the next address quantized by the given size.
//
//go:nosplit
func lookupnext(start uintptr, size uintptr) uintptr {
	start &= ^(size - 1)
	start += size
	return start
}
