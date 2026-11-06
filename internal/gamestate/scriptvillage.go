package gamestate

import (
	"encoding/binary"
	"math"
)

// Dialogue script functions 336–366: village posts, apprentices,
// buildings, repair, the curse, removal and vassalage. See
// docs/konung_dialogue.md#script-functions.

// Building records of a settlement (+0x18 + 8k) by role; records from 7
// on are the +0 extra buildings, then the +1 fence pieces.
const (
	BuildingHealer   = 1
	BuildingShop     = 2
	BuildingSmithy   = 3
	BuildingBarracks = 4
	BuildingBridge   = 5 // the map-exit gate
	BuildingWell     = 6
	buildingRecords  = (postOffset - 0x18) / 8
)

// Enchantments the scripts put on the hero's own slots (+0xD2, +0xD3).
const (
	enchantCurse  = 19 // «порча»: Сил −5 (344)
	questHareTail = 34 // quest item «Заячий хвост»: protects from the curse
)

func init() {
	canBuild := func(k int) func(*Talk) int {
		return func(t *Talk) int {
			s := t.settlement()
			if s == nil {
				return 0
			}
			req := k
			if k == BuildingShop {
				req = BuildingHealer // 0x00433798 reads record 1's type
			}
			return b2i(t.canStart(s, k, int(int8(s.building(req)[0])), true))
		}
	}
	startBuilding := func(k int) func(*Talk) int {
		return func(t *Talk) int {
			s := t.settlement()
			if s == nil {
				return 0
			}
			t.startTimer(s, k)
			return 1
		}
	}
	install := func(k int) func(*Talk) int { return func(t *Talk) int { return t.installAtPost(k) } }
	more := map[int]func(*Talk) int{
		336: func(t *Talk) int {
			return b2i(t.apprenticeCandidate() >= 0 && (!t.independent() || t.hero().Gold >= 100))
		},
		337: canBuild(BuildingHealer),
		338: canBuild(BuildingBarracks),
		339: canBuild(BuildingWell),
		340: canBuild(BuildingSmithy),
		341: canBuild(BuildingShop),
		342: canBuild(BuildingBridge),
		343: func(t *Talk) int {
			s := t.settlement()
			if s == nil || s.Fence == 0 {
				return 0
			}
			k := int(s.Raw[0]) + 7
			return b2i(k < buildingRecords && t.canStart(s, k, int(int8(s.building(k)[0])), false))
		},
		344: (*Talk).curse,
		345: install(4), 346: install(1), 347: install(3), 348: install(2),
		349: (*Talk).partyFull,
		350: func(t *Talk) int { return b2i(t.npc().Group == 0) },
		351: func(t *Talk) int { return b2i(t.npc().HP <= HPFull/2) },
		353: func(t *Talk) int { return b2i(t.npc().Flags4&0x38 != 0) },
		354: (*Talk).startApprentice,
		355: startBuilding(BuildingHealer),
		356: startBuilding(BuildingBarracks),
		357: startBuilding(BuildingWell),
		358: startBuilding(BuildingSmithy),
		359: startBuilding(BuildingShop),
		360: startBuilding(BuildingBridge),
		361: func(t *Talk) int {
			s := t.settlement()
			if s == nil {
				return 0
			}
			for k := int(s.Raw[0]) + 7; k < int(s.Raw[0])+7+s.Fence && k < buildingRecords; k++ {
				t.startTimer(s, k)
			}
			return 1
		},
		362: (*Talk).repair,
		363: (*Talk).vanish,
		364: func(t *Talk) int { t.hero().Enchants[0] = -1; return 1 },
		365: func(t *Talk) int {
			s := t.settlement()
			if s == nil {
				return 0
			}
			s.Flags &= settlementPeace
			binary.LittleEndian.PutUint32(s.Raw[0x10:], 0)
			binary.LittleEndian.PutUint32(s.Raw[0x14:], 0)
			return 1
		},
		366: func(t *Talk) int { return b2i(t.hero().Stats[0] > 60) },
	}
	for id, f := range more {
		scriptFuncs[id] = f
	}
}

// canStart is one of the "can build" checks (0x004334B8 …): record k is
// unstarted (state 0, timer 0; with typed, its type ≥ 0) and the village's
// best builder (0x00432520) has the skill type typ needs.
func (t *Talk) canStart(s *Settlement, k, typ int, typed bool) bool {
	r := s.building(k)
	if typed && int8(r[0]) < 0 {
		return false
	}
	return r[1] == 0 && binary.LittleEndian.Uint16(r[6:]) == 0 && t.G.maxBuildSkill(s.Group) >= t.T.BuildingSkill(typ)
}

// startTimer is 355–361: the record's timer +6 = the build time of its
// type (0x00458A66 + type·64). No cost; the settlement pass counts it
// down (buildingPass, 0x0041D939).
func (t *Talk) startTimer(s *Settlement, k int) {
	r := s.building(k)
	binary.LittleEndian.PutUint16(r[6:], uint16(t.T.BuildTime(int(int8(r[0])))))
}

// curse is 344 (0x00435874): unless the hero carries «Заячий хвост»
// (0x00434C90(0x23)), the hero's own enchantment +0xD2 = 19 (порча).
func (t *Talk) curse() int {
	if t.hasQuestItem(questHareTail) {
		return 0
	}
	t.hero().Enchants[0] = enchantCurse
	return 1
}

// partyFull is 349 (0x0043536C): the party's size limit (group +0x18 =
// 0x00411078(hero), MaxParty) is reached.
func (t *Talk) partyFull() int {
	g0 := &t.G.Groups[0]
	g0.Target = MaxParty(*t.hero(), t.T)
	return b2i(g0.Count >= g0.Target)
}

// apprenticeCandidate is 336's and 354's search: the first villager of the
// village group with a human body (+0xD6 < 3), no post bits (+4 & 0x38),
// alive, not of type 3/9 and without a post. −1 if none.
func (t *Talk) apprenticeCandidate() int {
	s := t.settlement()
	if s == nil {
		return -1
	}
	gr := t.G.Groups[s.Group]
	for k := range gr.Count {
		e := t.G.Entities[gr.First+k]
		if e.Model < 3 && e.Flags4&0x38 == 0 && !e.Dead() && e.Type != 3 && e.Type != 9 && !t.G.hasPost(e) {
			return gr.First + k
		}
	}
	return -1
}

// startApprentice is 354 (0x00435568): the NPC, holder of post 1–3, takes
// the first candidate as apprentice: its +4 gets the post (k << 3), the
// settlement remembers the start time (+0x3F8 + 4k, read by 291).
func (t *Talk) startApprentice() int {
	s := t.settlement()
	if s == nil {
		return 0
	}
	c := t.apprenticeCandidate()
	if c < 0 {
		return 0
	}
	k := 0
	for p := 1; p <= 3; p++ {
		if s.Posts[p].Entity == t.NPC {
			k = p
		}
	}
	if k == 0 {
		return 0
	}
	e := &t.G.Entities[c]
	e.Flags4 |= byte(k << 3)
	e.Flags2 = 0
	binary.LittleEndian.PutUint32(s.Raw[0x3F8+4*k:], uint32(int32(t.Time)))
	return 1
}

// installAtPost is 0x00432654(k) (345–348): with post k free, the first
// party member fit for it (0x004323B0) is dismissed into the village
// (0x00433AF8) and takes the post: its post bits clear, dialogue = the
// post's, stance off; it walks to the post cell (the scene's village AI
// takes it there).
func (t *Talk) installAtPost(k int) int {
	s := t.settlement()
	if s == nil || s.Posts[k].Entity != 0 {
		return 0
	}
	m := t.partyRole(k)
	if m == 0 {
		return 0
	}
	npc := t.NPC
	t.NPC = t.G.Groups[0].First + m
	ok := t.dismiss()
	t.NPC = npc
	if ok == 0 {
		return 0
	}
	vg := t.G.Groups[s.Group]
	idx := vg.First + vg.Count - 1
	e := &t.G.Entities[idx]
	e.Flags4 &^= 0x38
	e.Dialogue = s.Posts[k].Dialogue
	e.Flags3 &^= 0x04
	s.Posts[k].Entity = idx
	return 1
}

// repair is 362 (0x00435A84): the smith repairs the party's gear. For the
// six equipped items and the bag items of category ≤ 5 of every living
// member, the maximum condition (+8 float) above L = smithing·10 drops to
// L + (max − L)·0.7, and the condition (+4) becomes the maximum. (The
// skill gain after it never runs: it reads the repaired difference, 0.)
func (t *Talk) repair() int {
	n, g0 := t.npc(), t.G.Groups[0]
	limit := float64(n.Skills[1]) * 10
	fix := func(i int) {
		if i <= 0 || (i+1)*itemStride > len(t.G.items) {
			return
		}
		r := t.G.items[i*itemStride:]
		mx := float64(math.Float32frombits(binary.LittleEndian.Uint32(r[8:])))
		if mx > limit {
			mx = limit + (mx-limit)*0.7
		}
		binary.LittleEndian.PutUint32(r[8:], math.Float32bits(float32(mx)))
		binary.LittleEndian.PutUint32(r[4:], math.Float32bits(float32(mx)))
	}
	for k := range g0.Count {
		e := &t.G.Entities[t.Hero+k]
		if e.Dead() || e.Type == 3 || e.Type == 9 {
			continue
		}
		for _, i := range e.Equipment {
			fix(i)
		}
		for _, i := range e.Bag {
			if it, ok := t.G.Item(i); ok && i != 0 && it.Category <= 5 {
				fix(i)
			}
		}
	}
	return 1
}

// vanish is 363 (0x00434E98): the NPC leaves the world. Its post is
// cleared (later post indices drop by one only in that case, as in the
// game), its items are freed and its group closes the gap.
func (t *Talk) vanish() int {
	g, idx := t.G, t.NPC
	e := g.Entities[idx]
	if s := t.settlement(); s != nil && g.hasPost(e) {
		for p := range s.Posts {
			switch pe := s.Posts[p].Entity; {
			case pe == idx:
				s.Posts[p].Entity = 0
			case pe > idx:
				s.Posts[p].Entity--
			}
		}
	}
	g.freeItems(e)
	if e.Group >= 0 && e.Group < len(g.Groups) {
		g.removeMember(e.Group, idx, nil)
	}
	t.Events.PartyChanged = true
	return 1
}
