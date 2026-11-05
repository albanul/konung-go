package gamestate

// LeaveMap is the leave cleanup 0x00436AD0(map), run before another map
// loads. For each group on map m other than the party:
//
//   - every dead member (+4 bit 0x80) is removed: its items are freed
//     (0x00422778: quiver, equipment, bag and the five extra slots get
//     category 0xFF), the later members of the group move down one slot
//     (memmove), the count +0x1C drops, and when the group is the map's
//     settlement group (0x0043A1F8) its posts are fixed up (the dead one's
//     post emptied, higher entity indices decremented);
//   - then, if the settlement's post 0 (the elder) is empty, the first
//     living member with a human body (model < 3), +4 bits 0x38 clear and
//     no post (0x0041182C) takes it: post 0 = its index, its dialogue +0xD0 =
//     the post's dialogue (+0x3D1), +2 = 0.
//
// The game also loops over the party group; the port skips it (the party's
// death ends the game).
func (g *Game) LeaveMap(m int) {
	st := g.settlementOn(m)
	for gi := 1; gi < len(g.Groups); gi++ {
		gr := &g.Groups[gi]
		if gr.Map != m || gr.Kind == 0 {
			continue
		}
		village := st != nil && st.Group == gi
		for k := 0; k < gr.Count; k++ {
			idx := gr.First + k
			if idx < 0 || idx >= len(g.Entities) || !g.Entities[idx].Dead() {
				continue
			}
			g.freeItems(g.Entities[idx])
			if village {
				for p := range st.Posts {
					switch e := st.Posts[p].Entity; {
					case e == idx:
						st.Posts[p].Entity = 0
					case e > idx:
						st.Posts[p].Entity--
					}
				}
			}
			gr.Count--
			for j := idx; j < gr.First+gr.Count; j++ {
				g.Entities[j] = g.Entities[j+1]
				g.Entities[j].Index = j
			}
			k-- // the next member moved into this slot
		}
		if !village || st.Posts[0].Entity != 0 {
			continue
		}
		for k := 0; k < gr.Count; k++ {
			e := &g.Entities[gr.First+k]
			if e.Dead() || e.Model >= 3 || e.Flags4&0x38 != 0 || g.hasPost(*e) {
				continue
			}
			st.Posts[0].Entity = e.Index
			e.Dialogue = st.Posts[0].Dialogue
			e.Flags2 = 0
			break
		}
	}
}

// SettlementOnMap is 0x0043A1F8: the settlement record of map m, or nil.
func (g *Game) SettlementOnMap(m int) *Settlement { return g.settlementOn(m) }

// settlementOn is 0x0043A1F8: the settlement record of map m, or nil.
func (g *Game) settlementOn(m int) *Settlement {
	for i := range g.Settlements {
		if s := &g.Settlements[i]; s.Map == m && m != 0 {
			return s
		}
	}
	return nil
}

// hasPost reports whether e already holds a post in its settlement.
func (g *Game) hasPost(e Entity) bool {
	_, ok := g.PostOf(e)
	return ok
}

// freeItems is 0x00422778: every item e carries becomes a free record.
func (g *Game) freeItems(e Entity) {
	free := func(i int) {
		if i > 0 && (i+1)*itemStride <= len(g.items) {
			g.items[i*itemStride] = 0xFF
		}
	}
	free(e.Quiver)
	for _, i := range e.Equipment {
		free(i)
	}
	for _, i := range e.Bag {
		free(i)
	}
	for _, i := range e.Extra {
		free(i)
	}
}
