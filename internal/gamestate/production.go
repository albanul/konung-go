package gamestate

import (
	"encoding/binary"
	"math"

	"konung1.5/internal/resfmt"
)

// Village shop stock (docs/konung_trade.md#shop-stock-production). The
// healer and the smith produce in the settlement pass (0x0041D39F, every
// 16 ticks, all 20 settlements); the merchant's stock and the healer's
// jars are rolled when the map loads (0x00418F0C). Stock lists are item
// record indices in the settlement record: healer +0x412 (24 shown;
// production indexes a u16 array from +0x40C), merchant +0x444 (32), smith
// +0x484 (23).

func (s *Settlement) u16(off int) int      { return int(binary.LittleEndian.Uint16(s.Raw[off:])) }
func (s *Settlement) setU16(off, v int)    { binary.LittleEndian.PutUint16(s.Raw[off:], uint16(v)) }
func (s *Settlement) i32(off int) int      { return int(int32(binary.LittleEndian.Uint32(s.Raw[off:]))) }
func (s *Settlement) setI32(off, v int)    { binary.LittleEndian.PutUint32(s.Raw[off:], uint32(int32(v))) }
func (s *Settlement) healer(i int) int     { return s.u16(0x40C + 2*i) }
func (s *Settlement) setHealer(i, v int)   { s.setU16(0x40C+2*i, v) }
func (s *Settlement) merchant(i int) int   { return s.u16(0x444 + 2*i) }
func (s *Settlement) setMerchant(i, v int) { s.setU16(0x444+2*i, v) }
func (s *Settlement) smith(i int) int      { return s.u16(0x484 + 2*i) }
func (s *Settlement) setSmith(i, v int)    { s.setU16(0x484+2*i, v) }

// Settlement fields of the production.
const (
	healerTimerOff = 0x04  // int32: passes until the next healer cycle
	smithTimerOff  = 0x08  // int32: passes until the smith's job is done
	tributeOff     = 0x10  // int32: tribute pool (283, 311)
	tributeTickOff = 0x14  // int32: passes since the last tribute
	smithJobOff    = 0x4B4 // int8: the slot the smith works on, −1 = idle

	healerCycle   = 1440
	tributePeriod = 10080
	smithSlots    = 23
	merchantSlots = 32
	arrowStack    = 30
)

// rnd is the game's R(n) idiom: n < 2 gives 0 without calling rand().
func rnd(rand func(int) int, n int) int {
	if n < 2 {
		return 0
	}
	return rand(n)
}

// styleFixed is 0x00417220(x): the gear style (0..2) for a tribe offset.
func styleFixed(x int) int {
	switch (x + 1) & 3 {
	case 0:
		return 2
	case 1:
		return 1
	}
	return 0
}

// styleRandom is 0x00417170(tribe): one of two styles at random.
func styleRandom(rand func(int) int, tribe int) int {
	switch (rand(2) + tribe + 1) % 3 {
	case 0:
		return 2
	case 1:
		return 1
	}
	return 0
}

// newStock allocates an item (0x004399AC) of category cat and type typ.
func (g *Game) newStock(cat, typ int) int {
	i := g.newItem()
	if i != 0 {
		g.setItem(i, cat, typ)
	}
	return i
}

func (g *Game) setEnchant(i, e int) { g.items[i*itemStride+1] = byte(e) }

// SettlementPass is the settlement pass 0x0041D39F for every settlement
// (the surrender and alarm checks of the current map's village run in the
// scene, Surrenders and VillageAlarm). Per settlement: a village without
// humans calms down and loses its tribute counter, tribute accrual
// (0x0041D88A), buildings (0x0041D939), the healer (0x0041768C), the
// smith (0x00417A58), and when no building stage started, the voevoda's
// training (0x00418014). The game skips healer and smith while window 7
// (trade) is open: tradeOpen. m is the current map; changed reports a
// building there that changed state (BuildingObjects has the new IDs).
func (g *Game) SettlementPass(t *resfmt.ExeTables, hero, m int, tradeOpen bool) (changed bool) {
	for i := range g.Settlements {
		s := &g.Settlements[i]
		if s.Map == 0 {
			continue
		}
		humans, busy, posted := g.villageCount(s)
		if humans == 0 {
			g.Groups[s.Group].Alarm = AlarmCalm
			s.Flags &= settlementPeace
			s.setI32(tributeTickOff, 0)
		}
		g.tribute(s, hero)
		builders := 0
		if g.Groups[s.Group].Alarm != AlarmAttack {
			builders = humans - (busy + posted)
		}
		started, ch := g.buildingPass(s, t, builders, m)
		changed = changed || ch
		if !tradeOpen {
			g.healerPass(s, t)
			g.smithPass(s, t)
		}
		if !started {
			g.voevodaPass(s, t)
		}
	}
	return changed
}

// tribute is 0x0041D88A: a vassal village (not independent) adds to its
// tribute pool every 10080 passes: extra buildings · 20 · √(hero's trade)
// + members · 5 + 1.
func (g *Game) tribute(s *Settlement, hero int) {
	if s.Flags&SettlementIndependent != 0 {
		return
	}
	n := s.i32(tributeTickOff) + 1
	if n >= tributePeriod {
		trade := float64(uint16(g.Entities[hero].Skills[2]))
		add := float64(s.Raw[0])*20*math.Sqrt(trade) + float64(g.Groups[s.Group].Count*5+1)
		s.setI32(tributeOff, int(math.Trunc(float64(s.i32(tributeOff))+add)))
		n = 0
	}
	s.setI32(tributeTickOff, n)
}

// healerPass is 0x0041768C. Flags H[0..2] allow one Яд, Масло, Бальзам;
// the timer (+4) refills them: all three at the cycle start (1440
// passes), Масло and Бальзам at 720, Бальзам at 1080 and 360. Slots
// 5–7 and 19–21 get a fresh base potion (a new herb mixed into a jar,
// 0x0041BC24); slots 8–13 and 22–27 combine two of the potions in 19–23
// (0x00417268).
func (g *Game) healerPass(s *Settlement, t *resfmt.ExeTables) {
	id := s.Posts[1].Entity
	if id == 0 {
		return
	}
	timer := s.i32(healerTimerOff) - 1
	switch {
	case timer <= 0:
		timer = healerCycle
		s.setHealer(0, 1)
		s.setHealer(1, 1)
		s.setHealer(2, 1)
	case timer == 720:
		s.setHealer(1, 1)
		s.setHealer(2, 1)
	case timer == 1080 || timer == 360:
		s.setHealer(2, 1)
	}
	s.setI32(healerTimerOff, timer)
	e := &g.Entities[id]
	base := func(i, flag, herb int) bool {
		if s.healer(flag) == 0 {
			return true
		}
		it := g.newStock(CatHerb, herb)
		if it == 0 {
			return false
		}
		// The game mixes with a jar template on the stack; the port uses
		// a scratch record and frees it again, so the indices match.
		jar := g.newStock(CatHerb, HerbJar)
		if jar == 0 {
			g.items[it*itemStride] = 0xFF
			return false
		}
		g.Mix(e, t, it, jar)
		g.items[jar*itemStride] = 0xFF
		s.setHealer(i, it)
		s.setHealer(flag, s.healer(flag)-1)
		return true
	}
	mix := func(i, a, b int) {
		A, B := s.healer(a), s.healer(b)
		if A == 0 || B == 0 {
			return
		}
		g.Mix(e, t, A, B)
		g.items[B*itemStride] = 0xFF
		s.setHealer(i, A)
		s.setHealer(a, 0)
		s.setHealer(b, 0)
	}
	for i := 5; i <= 27; i++ {
		if s.healer(i) != 0 {
			continue
		}
		ok := true
		switch i {
		case 5, 19:
			ok = base(i, 2, HerbRoot)
		case 6, 20:
			ok = base(i, 0, HerbSting)
		case 7, 21:
			ok = base(i, 1, HerbNut)
		case 8, 22:
			mix(i, 19, 20)
		case 9, 23:
			mix(i, 19, 21)
		case 10, 24:
			mix(i, 19, 23)
		case 11, 25:
			mix(i, 20, 21)
		case 12, 26:
			mix(i, 20, 23)
		case 13, 27:
			mix(i, 21, 22)
		}
		if !ok {
			return // alloc failed: the routine returns
		}
	}
}

// smithSlot is what smith slot i makes for style k: arrows (types 30–32,
// 30 a stack), then the style's 3 blades, 3 axes, 3 bows, 3 armours, 4
// shields and 4 helmets.
func smithSlot(i, k int) (cat, typ int) {
	switch {
	case i < 3:
		return CatArrows, 30 + i
	case i < 12:
		typ = 9*k + i - 3
		cat = []int{CatBlade, CatAxe, CatBow}[(i-3)/3]
		return cat, typ
	}
	typ = 11*k + 0x22 + i - 0xC
	switch {
	case i < 15:
		return CatArmour, typ
	case i < 19:
		return CatShield, typ
	}
	return CatHelmet, typ
}

// smithPass is 0x00417A58. Idle (+0x4B4 < 0): the first empty slot whose
// work value W (gear +0x2E) the smith masters (smithing · 10 ≥ W) becomes
// the job, taking W · 60 / √smithing passes (+8). With every mastered slot
// stocked, the first stocked item a village member can use and lacks (an
// empty quiver for arrows; an empty or lower-type equipment slot, whose old
// item is destroyed) is handed over and its slot restarted. Working: when
// the timer runs out the item is made (no enchantment) and the smith's
// skill becomes trunc(√(skill² + (trunc(W / √skill) + 1) / 24)).
func (g *Game) smithPass(s *Settlement, t *resfmt.ExeTables) {
	id := s.Posts[3].Entity
	if id == 0 {
		return
	}
	k := styleFixed(int(g.Groups[s.Group].Kind&3) - 1)
	smith := &g.Entities[id]
	skill := float64(uint16(smith.Skills[1]))
	work := func(i int) int {
		_, typ := smithSlot(i, k)
		return t.ItemField16(resfmt.ItemTableGear, typ, 0x2E)
	}
	start := func(i, w int) {
		s.Raw[smithJobOff] = byte(i)
		s.setI32(smithTimerOff, int(math.Trunc(float64(w*60)/math.Sqrt(skill))))
	}
	if int8(s.Raw[smithJobOff]) < 0 {
		var full []int
		for i := range smithSlots {
			w := work(i)
			if skill*10 < float64(w) {
				continue
			}
			if s.smith(i) == 0 {
				start(i, w)
				return
			}
			full = append(full, i)
		}
		vg := g.Groups[s.Group]
		for _, i := range full {
			cat, typ := smithSlot(i, k)
			it := s.smith(i)
			item, _ := g.Item(it)
			for m := range vg.Count {
				e := &g.Entities[vg.First+m]
				if !g.CanUse(*e, t, item) {
					continue
				}
				if i < 3 {
					if e.Quiver != 0 {
						continue
					}
					e.Quiver = it
				} else {
					cur := e.Equipment[cat]
					if c, ok := g.Item(cur); cur != 0 && ok && c.Type >= typ {
						continue
					}
					if cur != 0 {
						g.items[cur*itemStride] = 0xFF
					}
					e.Equipment[cat] = it
				}
				s.setSmith(i, 0)
				start(i, work(i))
				return
			}
		}
		return
	}
	timer := s.i32(smithTimerOff) - 1
	s.setI32(smithTimerOff, timer)
	if timer > 0 {
		return
	}
	i := int(int8(s.Raw[smithJobOff]))
	cat, typ := smithSlot(i, k)
	it := g.newStock(cat, typ)
	if it == 0 {
		return
	}
	s.Raw[smithJobOff] = 0xFF
	s.setSmith(i, it)
	if i < 3 {
		binary.LittleEndian.PutUint32(g.items[it*itemStride+4:], arrowStack)
	} else {
		g.setCondition(it, t)
	}
	v := math.Trunc(float64(work(i))/math.Sqrt(skill) + 1)
	smith.Skills[1] = int(math.Trunc(math.Sqrt(skill*skill + v/24)))
}

// Restock is 0x00418F0C, run by the map loader (also when a save is
// restored) for the settlement of map m: 1–3 empty jars for the healer
// (slots +0x412 and +0x42E, when empty), then the merchant's stock: slots
// 0–13 are filled when empty, each by chance; slots 14–31 are replaced by
// 18 new items. Types depend on the hero's level L and the village's
// tribe; see docs/konung_trade.md.
func (g *Game) Restock(m, hero int, t *resfmt.ExeTables, rand func(int) int) {
	s := g.settlementOn(m)
	if s == nil {
		return
	}
	n := rand(3) + 1
	for _, i := range []int{3, 17} {
		if n != 0 && s.healer(i) == 0 {
			it := g.newStock(CatHerb, HerbJar)
			if it == 0 {
				return
			}
			n--
			s.setHealer(i, it)
		}
	}
	L := g.Entities[hero].Level
	tribe := int(g.Groups[s.Group].Kind & 3)
	R := func(n int) int { return rnd(rand, n) }
	ej := func() int { return 3*R(L/8+1) + 10*(rand(4)+1) + rand(2) }
	ew0 := func() int { return 3*R(L/8+1) + 10*rand(4) + rand(2) }
	wcat := func(t int) int {
		if t%9 < 3 {
			return CatBlade
		}
		return CatAxe
	}
	gear := func(cat, typ int) int {
		it := g.newStock(cat, typ)
		if it != 0 {
			g.setCondition(it, t)
		}
		return it
	}
	for i := 0; i < 14; i++ {
		if s.merchant(i) != 0 {
			continue
		}
		var chance int
		switch {
		case i < 3:
			chance = 70
		case i < 5:
			chance = 50
		case i < 8:
			chance = 30
		default:
			chance = 50
		}
		if rand(100) <= chance {
			continue
		}
		it := g.newItem()
		if it == 0 {
			return
		}
		switch {
		case i < 3:
			r := rand(30)
			switch {
			case r < 10:
				g.setItem(it, CatRing, 6+R(L/5))
			case r < 20:
				g.setItem(it, CatBracelet, 3+R(L/5))
			default:
				g.setItem(it, CatNecklace, R(L/5))
			}
			g.setEnchant(it, ej())
		case i < 5:
			g.setItem(it, CatQuest, rand(10))
		case i < 8:
			typ := R(L/5+1) + 9*styleRandom(rand, tribe) + 3*rand(2)
			g.setItem(it, wcat(typ), typ)
			g.setCondition(it, t)
			if rand(101) < 10 {
				g.setEnchant(it, R(lootLevelBand(L))+10*rand(5))
			}
		default:
			switch rand(3) {
			case 0:
				typ := R(L/5+1) + 11*styleRandom(rand, tribe) + 0x22
				g.setItem(it, CatArmour, typ)
			case 1:
				typ := R(L/4+1) + 11*styleRandom(rand, tribe) + 0x25
				g.setItem(it, CatShield, typ)
			default:
				typ := R(L/4+1) + 11*styleRandom(rand, tribe) + 0x29
				g.setItem(it, CatHelmet, typ)
			}
			g.setCondition(it, t)
			if rand(100) < 7 {
				g.setEnchant(it, R(lootLevelBand(L))+10*rand(5))
			}
		}
		s.setMerchant(i, it)
	}
	// 0x004197B2: the regenerated half. The game frees the old items but
	// keeps their indices until overwritten; the port clears them so an
	// allocation failure leaves no dangling entries.
	for i := 14; i < merchantSlots; i++ {
		if it := s.merchant(i); it != 0 {
			g.items[it*itemStride] = 0xFF
			s.setMerchant(i, 0)
		}
	}
	T0, T1 := styleFixed(tribe), styleFixed(tribe+1)
	put := func(i, it int) bool {
		if it == 0 {
			return false
		}
		s.setMerchant(i, it)
		return true
	}
	jewel := func(cat, typ int) int {
		it := g.newStock(cat, typ)
		if it != 0 {
			g.setEnchant(it, ej())
		}
		return it
	}
	weapon := func(style int) int {
		typ := R(L/5+1) + 9*style + 3*rand(2)
		return gear(wcat(typ), typ)
	}
	steps := []func() int{
		func() int { return jewel(CatBracelet, 3+R(L/5)) },
		func() int { return jewel(CatNecklace, R(L/5)) },
		func() int { return jewel(CatRing, 6+R(L/5)) },
		func() int { return weapon(T0) },
		func() int { return weapon(T1) },
		func() int { return gear(CatBow, 6+R(L/5+1)+9*T0) },
		func() int { return gear(CatBow, 6+R(L/5+1)+9*T1) },
		func() int { return gear(CatArmour, 0x22+R(L/5+1)+11*T0) },
		func() int { return gear(CatShield, 0x25+R(L/4+1)+11*T0) },
		func() int { return gear(CatHelmet, 0x29+R(L/4+1)+11*T0) },
		func() int { return gear(CatArmour, 0x22+R(L/5+1)+11*T1) },
		func() int { return gear(CatShield, 0x25+R(L/4+1)+11*T1) },
		func() int { return gear(CatHelmet, 0x29+R(L/4+1)+11*T1) },
		func() int {
			typ := R(L/5+1) + 9*rand(3) + 3*rand(2)
			it := gear(wcat(typ), typ)
			if it != 0 {
				g.setEnchant(it, ew0())
			}
			return it
		},
		func() int {
			it := gear(CatBow, 6+R(L/5+1)+9*rand(3))
			if it != 0 {
				g.setEnchant(it, ew0())
			}
			return it
		},
		func() int { return enchanted(g, gear(CatArmour, 0x22+R(L/5+1)+11*rand(3)), ej) },
		func() int { return enchanted(g, gear(CatShield, 0x25+R(L/4+1)+11*rand(3)), ej) },
		func() int { return enchanted(g, gear(CatHelmet, 0x29+R(L/4+1)+11*rand(3)), ej) },
	}
	for k, f := range steps {
		if !put(14+k, f()) {
			return
		}
	}
}

func enchanted(g *Game, it int, e func() int) int {
	if it != 0 {
		g.setEnchant(it, e())
	}
	return it
}
