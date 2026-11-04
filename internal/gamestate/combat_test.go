package gamestate

import (
	"encoding/binary"
	"math"
	"testing"

	"konung1.5/internal/resfmt"
)

// TestMeleeGame0 checks the melee formulas with Volk (knife, Фехтование 1.00)
// against giant ant 921 on KONUNG.1 (Лов 1, Сил 8, Вын 3, armour 10).
func TestMeleeGame0(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	volk := g.Entities[g.Groups[0].First]
	ant := g.Entities[921]
	if got := g.HitChance(volk, tb, 0); got != 37 { // trunc(100 − 95/1.515)
		t.Errorf("Volk hit chance %d, want 37", got)
	}
	if got := g.HitChance(ant, tb, 0); got != 50 { // trunc(100 − 99.5·0.5)
		t.Errorf("ant hit chance %d, want 50", got)
	}
	if got := g.KillExperience(ant, tb); got != 4 { // trunc(1 + 0.2 + 2.4 + 0.9)
		t.Errorf("ant experience %d, want 4", got)
	}
	// Facing 0/0 never dodges (resfmt.FacingDodge); 120 attack against
	// armour 10, Вын 3: trunc(((36 + 74)/3 + 1)·16) = 602.
	if tb.FacingDodge(0, 0) || !tb.FacingDodge(0, 4) {
		t.Fatal("facing table 0x00450E14 not as expected")
	}
	knife := volk.Equipment[WeaponBlade]
	cond := func() float32 {
		return math.Float32frombits(binary.LittleEndian.Uint32(g.items[knife*itemStride+4:]))
	}
	before := cond()
	volk.Variant, ant.Variant = 0, 0
	r, dmg := g.Strike(&volk, &ant, tb, func(int) int { return 0 })
	if r != StrikeHeavy || dmg != 602 || ant.HP != 1600-602 {
		t.Fatalf("strike = %d, %d dmg, ant HP %d; want heavy hit, 602, 998", r, dmg, ant.HP)
	}
	if d := before - cond(); math.Abs(float64(d)-120*0.00025) > 1e-6 {
		t.Errorf("knife wore by %v, want 0.03", d)
	}
	if r, _ := g.Strike(&volk, &ant, tb, func(n int) int { return n - 1 }); r != StrikeMiss {
		t.Errorf("roll 100 against 37 %% gave %d, want a miss", r)
	}
	ant.Variant = 4 // facing Volk: dodges when the roll is within trunc(Лов·0.5) = 0
	if r, _ := g.Strike(&volk, &ant, tb, func(int) int { return 0 }); r != StrikeDodged {
		t.Errorf("facing attacker with roll 0 gave %d, want a dodge", r)
	}
	ant.Variant = 0
	g.Strike(&volk, &ant, tb, func(int) int { return 0 })
	if r, _ := g.Strike(&volk, &ant, tb, func(int) int { return 0 }); r != StrikeKill {
		t.Fatalf("third hit on %d HP gave %d, want a kill", ant.HP, r)
	}
	free, fencing := volk.FreeExp, volk.Fencing
	if xp := g.Kill(&volk, &ant, tb); xp != 4 || ant.HP != 0 || volk.FreeExp != free+1 || volk.Fencing != fencing+4 {
		t.Errorf("kill: xp %d, ant HP %d, free exp +%d, Фехтование +%d", xp, ant.HP, volk.FreeExp-free, volk.Fencing-fencing)
	}
}
