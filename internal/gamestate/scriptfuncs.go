package gamestate

import (
	"encoding/binary"
	"math"
)

// Dialogue script functions from the pointer table 0x004657F4 (ids ≥
// 0x112), used as conditions (non-zero = true) and actions. See
// docs/konung_dialogue.md#script-functions for the full list; ids missing
// here are either unused by QUESTR.RES or not ported (Talk.script records
// them in Events.Unported).
var scriptFuncs map[int]func(*Talk) int

func init() {
	gold := func(d int) func(*Talk) int {
		return func(t *Talk) int { t.hero().Gold += d; return 1 }
	}
	goldAtLeast := func(n int) func(*Talk) int {
		return func(t *Talk) int { return b2i(t.hero().Gold >= n) }
	}
	exp := func(n int) func(*Talk) int {
		return func(t *Talk) int { t.G.GainExperience(t.hero(), t.T, n); return 1 }
	}
	partyRole := func(k int) func(*Talk) int {
		return func(t *Talk) int { return b2i(t.partyRole(k) != 0) }
	}
	post := func(k int) func(*Talk) int {
		return func(t *Talk) int { s := t.settlement(); return b2i(s != nil && s.Posts[k].Entity != 0) }
	}
	building := func(k int) func(*Talk) int {
		return func(t *Talk) int {
			s := t.settlement()
			if s == nil {
				return 0
			}
			r := s.building(k)
			return b2i(int8(r[1]) > 0 || int16(binary.LittleEndian.Uint16(r[6:])) > 0)
		}
	}
	herb := func(typ int) func(*Talk) int {
		return func(t *Talk) int {
			for _, i := range t.hero().Bag {
				if it, ok := t.G.Item(i); ok && i != 0 && it.Category == CatHerb && it.Type == typ {
					return 1
				}
			}
			return 0
		}
	}
	scriptFuncs = map[int]func(*Talk) int{
		274: (*Talk).attack,
		275: func(t *Talk) int { t.Events.Trade = t.G.OpenTrade(t.Hero, t.Hero, t.NPC, t.Map); return 1 },
		276: func(t *Talk) int { t.hero().Enchants[1] = enchantBlessing; return 1 },
		277: func(t *Talk) int { return b2i(t.rand(100) < 50) },
		278: partyRole(4), 279: partyRole(1), 280: partyRole(3), 281: partyRole(2),
		282: (*Talk).repayLoan,
		283: (*Talk).collectTribute,
		284: (*Talk).takeLoan,
		285: gold(-5), 286: gold(-10), 287: gold(-20), 288: gold(-50), 289: gold(-100), 290: gold(-250),
		291: (*Talk).recruit,
		293: gold(5), 294: gold(10), 295: gold(20), 296: gold(50), 297: gold(100), 298: gold(250),
		299: exp(10), 300: exp(20), 301: exp(50),
		302: func(t *Talk) int { return b2i(t.independent()) },
		303: func(t *Talk) int { return b2i(t.hero().Enchants[1] >= 0) },
		304: post(4),
		305: goldAtLeast(30), 307: goldAtLeast(50), 308: goldAtLeast(100), 309: goldAtLeast(500),
		310: (*Talk).canPayHire,
		311: func(t *Talk) int { s := t.settlement(); return b2i(s != nil && s.Tribute() != 0) },
		312: func(t *Talk) int { return b2i(t.settlement() != nil) },
		313: herb(HerbNut),
		314: post(1),
		315: building(1),
		316: building(4),
		317: building(6),
		318: herb(HerbRoot),
		319: post(3),
		320: building(3),
		321: post(2),
		322: building(2),
		324: (*Talk).hasUnidentified,
		325: func(t *Talk) int { return b2i(t.hero().Enchants[0] >= 0) },
		326: func(t *Talk) int {
			s := t.settlement()
			return b2i(s != nil && t.G.maxBuildSkill(s.Group) >= 5)
		},
		328: (*Talk).canBuild,
		329: herb(HerbSting),
		330: func(t *Talk) int { return b2i(t.hero().HP < HPFull) },
		331: (*Talk).identifyAll,
		332: (*Talk).dismiss,
		333: func(t *Talk) int { s := t.settlement(); return b2i(s != nil && s.Flags&SettlementLoan != 0) },
		334: (*Talk).healHero,
		335: (*Talk).healParty,
		352: func(t *Talk) int { return b2i(t.npc().Level >= 4) },
	}
}

// enchantBlessing is the enchantment 276 puts on the hero (+0xD3 = 70):
// requirement Вын ≥ 30, Хар +10 (0x00458590 + 70·16).
const enchantBlessing = 70

// SettlementLoan is +0x4B2 bit 0x20: the village lent the hero money (284,
// repaid by 282, tested by 333).
const SettlementLoan = 0x20

func (t *Talk) npc() *Entity { return &t.G.Entities[t.NPC] }

// independent is 302 (0x00435460): the current village is not the
// player's (+0x4B2 bit 0x10).
func (t *Talk) independent() bool {
	s := t.settlement()
	return s != nil && s.Flags&SettlementIndependent != 0
}

// Tribute is the settlement's tribute pool (+0x10 int32, 283, 311).
func (s *Settlement) Tribute() int { return int(int32(binary.LittleEndian.Uint32(s.Raw[0x10:]))) }

func (s *Settlement) setTribute(v int) { binary.LittleEndian.PutUint32(s.Raw[0x10:], uint32(int32(v))) }

// building returns building record k (+0x18 + 8k): int8 type, state, …,
// int16 timer +6.
func (s *Settlement) building(k int) []byte { return s.Raw[0x18+8*k:][:8] }

// attack is 274 (0x00432F70): the NPC's group is alarmed (+0x1D = 1); when
// it is the village group the village turns hostile (+0x4B2 |= 0x15).
func (t *Talk) attack() int {
	gi := t.npc().Group
	if gi < 0 || gi >= len(t.G.Groups) {
		return 1
	}
	t.G.Groups[gi].Alarm = AlarmAttack
	t.G.Groups[gi].Enemy = 0
	if s := t.settlement(); s != nil && s.Group == gi {
		s.Flags |= settlementAnger
	}
	t.Events.Hostile = true
	return 1
}

// partyRole is 0x004323B0(k): the 1-based index of the first living party
// member after the hero with role k (+4 & 0x38 = k<<3) or the skill of
// that role: 1 healer (healing > 100), 2 merchant (trade > 30), 3 smith
// (smithing > 4), 4 warrior (Сил > 10 and fencing > 10); 0 if none.
func (t *Talk) partyRole(k int) int {
	g0 := t.G.Groups[0]
	for m := 1; m < g0.Count; m++ {
		e := t.G.Entities[g0.First+m]
		if e.Dead() {
			continue
		}
		var skill bool
		switch k {
		case 1:
			skill = e.Skills[0] > 100
		case 2:
			skill = e.Skills[2] > 30
		case 3:
			skill = e.Skills[1] > 4
		case 4:
			skill = e.Stats[1] > 10 && e.Fencing > 10
		}
		if int(e.Flags4&0x38) == k<<3 || skill {
			return m
		}
	}
	return 0
}

// repayLoan is 282 (0x00435D6C): with at least 120 gold, pay 120 and
// clear the loan.
func (t *Talk) repayLoan() int {
	s, h := t.settlement(), t.hero()
	if s == nil || h.Gold < 120 {
		return 0
	}
	s.Flags &^= SettlementLoan
	h.Gold -= 120
	return 1
}

// takeLoan is 284 (0x00434A28): +100 gold and the loan flag; returns 0.
func (t *Talk) takeLoan() int {
	if s := t.settlement(); s != nil {
		s.Flags |= SettlementLoan
		t.hero().Gold += 100
	}
	return 0
}

// collectTribute is 283 (0x00434C44): the tribute pool goes to the hero.
func (t *Talk) collectTribute() int {
	s := t.settlement()
	if s == nil {
		return 0
	}
	t.hero().Gold += s.Tribute()
	s.setTribute(0)
	return 1
}

// canPayHire is 310 (0x004343D0): the hero can pay the NPC's hire price,
// 500 from level 4 (0x004354A8) else 300, halved unless 302.
func (t *Talk) canPayHire() int {
	price := 300
	if t.npc().Level >= 4 {
		price = 500
	}
	if !t.independent() {
		price /= 2
	}
	return b2i(t.hero().Gold >= price)
}

// heroItems lists every item the hero carries: equipment, ammunition,
// bag and jewellery (0x00434438, 0x0041BABC).
func heroItems(e *Entity) []*int {
	out := []*int{}
	for i := range e.Equipment {
		out = append(out, &e.Equipment[i])
	}
	out = append(out, &e.Quiver)
	for i := range e.Bag {
		out = append(out, &e.Bag[i])
	}
	for i := range e.Extra {
		out = append(out, &e.Extra[i])
	}
	return out
}

// hasUnidentified is 324 (0x00434438).
func (t *Talk) hasUnidentified() int {
	for _, p := range heroItems(t.hero()) {
		if it, ok := t.G.Item(*p); ok && *p != 0 && it.Flagged {
			return 1
		}
	}
	return 0
}

// identifyAll is 331 (0x00435100 → 0x0041BABC): every unidentified item
// the hero carries is identified, each adding 1 to the hero's curios skill
// (+0x4A), capped at 100.
func (t *Talk) identifyAll() int {
	h := t.hero()
	for _, p := range heroItems(h) {
		if it, ok := t.G.Item(*p); ok && *p != 0 && it.Flagged {
			t.G.items[*p*itemStride+0xE] ^= 1
			h.Skills[4]++
		}
	}
	h.Skills[4] = min(h.Skills[4], 100)
	return 1
}

// healGain is the healer's skill gain for healing e: trunc((1600 − HP)·0.015).
func healGain(e Entity) float64 { return float64(HPFull-e.HP) * 0.015 }

// healHero is 334 (0x00434D1C): the NPC's healing skill grows, the hero's
// HP is full.
func (t *Talk) healHero() int {
	n, h := t.npc(), t.hero()
	n.Skills[0] = int(math.Trunc(healGain(*h) + float64(n.Skills[0])))
	h.HP = HPFull
	return 1
}

// healParty is 335 (0x00434DA4): the same for every living party member.
func (t *Talk) healParty() int {
	n, g0 := t.npc(), t.G.Groups[0]
	for k := range g0.Count {
		e := &t.G.Entities[t.Hero+k]
		if e.Dead() || e.Type == 3 || e.Type == 9 {
			continue
		}
		n.Skills[0] += int(math.Trunc(healGain(*e)))
		e.HP = HPFull
	}
	return 1
}

// maxBuildSkill is 0x00432520(group): the best building skill (+0x46) of
// the group's members.
func (g *Game) maxBuildSkill(gi int) int {
	best, gr := 0, g.Groups[gi]
	for k := range gr.Count {
		best = max(best, g.Entities[gr.First+k].Skills[3])
	}
	return best
}

// canBuild is 328 (0x00434570): any of seven buildings can be started: its
// record has a type (≥ 0), state 0, timer 0, and the village's best
// builder meets the type's skill (building table 0x00458A60 + type·64,
// +2). Records 3 (forge), 5 (gate), the fence piece (+0 + 7, fenced
// villages only, no type check), 1, 2 (with record 1's requirement, as in
// 0x00433798), 4 and 6.
func (t *Talk) canBuild() int {
	s := t.settlement()
	if s == nil {
		return 0
	}
	best := t.G.maxBuildSkill(s.Group)
	need := func(typ int) int { return t.T.BuildingSkill(typ) }
	free := func(r []byte) bool { return r[1] == 0 && binary.LittleEndian.Uint16(r[6:]) == 0 }
	check := func(k, req int, typed bool) bool {
		r := s.building(k)
		return (!typed || int8(r[0]) >= 0) && free(r) && best >= need(req)
	}
	typeOf := func(k int) int { return int(int8(s.building(k)[0])) }
	switch {
	case check(3, typeOf(3), true), check(5, typeOf(5), true):
		return 1
	case s.Fence != 0 && int(s.Raw[0])+7 < 119 && check(int(s.Raw[0])+7, typeOf(int(s.Raw[0])+7), false):
		return 1
	case check(1, typeOf(1), true), check(2, typeOf(1), true), check(4, typeOf(4), true), check(6, typeOf(6), true):
		return 1
	}
	return 0
}

// recruit is 291 (0x00432820): the NPC joins the party. A trainee (+4 &
// 0x38 = role k) first gains the skill of the days it trained (time since
// settlement +0x3F8 + 4k, 1440·16 ticks a day); it is appended to group 0
// (+5 = 0, +2 = 0x60, +0x14 = 0xFF, a portrait variant +0xF8 when 0), and
// removed from its old group, whose later members move down; the posts
// are fixed up.
func (t *Talk) recruit() int {
	g, n := t.G, t.NPC
	e := g.Entities[n]
	s := t.settlement()
	if k := int(e.Flags4&0x38) >> 3; k != 0 && s != nil && k <= 3 {
		days := float64((t.Time - int(int32(binary.LittleEndian.Uint32(s.Raw[0x3F8+4*k:])))) / 16 / 1440)
		master := func(post, skill int) (int, bool) {
			if m := s.Posts[post].Entity; m != 0 {
				return g.Entities[m].Skills[skill], true
			}
			return 0, false
		}
		grow := func(skill, post, limit int) {
			v := float64(e.Skills[skill])
			if m, ok := master(post, skill); ok {
				v += float64(m-e.Skills[skill]) * 0.03 * days
			} else {
				v += days * 0.03
			}
			e.Skills[skill] = int(math.Trunc(v))
			if limit > 0 {
				e.Skills[skill] = min(e.Skills[skill], limit)
			}
		}
		switch k {
		case 1:
			grow(0, 1, 500)
		case 2:
			e.Skills[2] += int(days)
			if m, ok := master(2, 2); ok && m < e.Skills[2] {
				e.Skills[2] = m
			}
			e.Skills[2] = min(e.Skills[2], 100)
		case 3:
			grow(1, 3, 0)
		}
	}
	g0 := &g.Groups[0]
	dst := g0.First + g0.Count
	if dst >= len(g.Entities) {
		return 0
	}
	g0.Count++
	e.Group, e.Flags2 = 0, 0x60
	e.Raw[0x14] = 0xFF
	if binary.LittleEndian.Uint16(e.Raw[0xF8:]) == 0 {
		t.pickFace(&e)
	}
	old := g.Entities[n].Group
	e.Index = dst
	g.Entities[dst] = e
	if old > 0 && old < len(g.Groups) {
		g.removeMember(old, n, s)
	}
	t.Events.PartyChanged = true
	return 1
}

// pickFace is 291's portrait variant +0xF8: (model·36 + (f−1)·9 + r)·5 + 1
// with f = +4 & 7 (5 → 4) and r the first variant from rand() % 9 that no
// other party member of the same model and face uses.
func (t *Talk) pickFace(e *Entity) {
	f := int(e.Flags4 & 7)
	if f == 5 {
		f = 4
	}
	m := e.Model
	base := (m*4 + f - 1) * 45
	var used [9]bool
	g0 := t.G.Groups[0]
	for k := range g0.Count - 1 {
		o := t.G.Entities[g0.First+k]
		of := int(o.Flags4 & 7)
		if of == 5 {
			of = 4
		}
		if of == f && o.Model == m {
			if v := (int(binary.LittleEndian.Uint16(o.Raw[0xF8:])) - 1 - base) / 5; v >= 0 && v < 9 {
				used[v] = true
			}
		}
	}
	r := t.rand(9)
	for i := 0; i < 9 && used[r]; i++ {
		r = (r + 1) % 9
	}
	binary.LittleEndian.PutUint16(e.Raw[0xF8:], uint16((m*36+(f-1)*9+r)*5+1))
}

// removeMember takes entity idx out of group gi: the later members move
// down one slot and the settlement's posts are fixed up (0x00432820,
// 0x00433AF8, like LeaveMap).
func (g *Game) removeMember(gi, idx int, s *Settlement) {
	gr := &g.Groups[gi]
	end := gr.First + gr.Count
	gr.Count--
	for j := idx; j < end-1; j++ {
		g.Entities[j] = g.Entities[j+1]
		g.Entities[j].Index = j
	}
	if s != nil && gi != 0 {
		for p := range s.Posts {
			switch e := s.Posts[p].Entity; {
			case e == idx:
				s.Posts[p].Entity = 0
			case e > idx:
				s.Posts[p].Entity--
			}
		}
	}
}

// dismiss is 332 (0x00433AF8): a party member stays in the village. It
// fails when there is no village or its group is full; otherwise the NPC
// is appended to the village group (+2 = 0x60, +0x14 = 0xFF), takes an
// empty elder post (its dialogue becomes the post's, +2 = 0, +4 &= 0xC7)
// and leaves the party, whose later members move down.
func (t *Talk) dismiss() int {
	s := t.settlement()
	if s == nil {
		return 0
	}
	g := t.G
	vg := &g.Groups[s.Group]
	if vg.Count >= vg.MaxPop {
		return 0
	}
	e := g.Entities[t.NPC]
	dst := vg.First + vg.Count
	vg.Count++
	e.Index, e.Group, e.Flags2 = dst, s.Group, 0x60
	e.Raw[0x14] = 0xFF
	if s.Posts[0].Entity == 0 {
		s.Posts[0].Entity = dst
		e.Dialogue = s.Posts[0].Dialogue
		e.Flags2 = 0
		e.Flags4 &= 0xC7
	}
	g.Entities[dst] = e
	g.removeMember(0, t.NPC, nil)
	t.Events.PartyChanged = true
	return 1
}
