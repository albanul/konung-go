package gamestate

import "testing"

func TestCreatureRulesGame0(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	// Group 4: giant ants (kind 0x41) on KONUNG.1, territory rows 4-36 cols
	// 12-20 inside the leash rows 0-46 cols 2-25.
	gr := g.Groups[4]
	in, _ := gr.WanderArea()
	if !gr.CreatureGroup() || in != (Area{4, 36, 12, 20}) || gr.LeashArea() != (Area{0, 46, 2, 25}) || gr.Alarm != 0x80 {
		t.Fatalf("group 4: creature %v, territory %+v, leash %+v, alarm %#x", gr.CreatureGroup(), in, gr.LeashArea(), gr.Alarm)
	}
	inside, outside := [][2]int{{10, 15}}, [][2]int{{40, 15}}
	if s := gr.AlarmState(inside, outside); s != AlarmAttack {
		t.Errorf("party inside: %#x, want attack", s)
	}
	if s := gr.AlarmState(outside, outside); s != AlarmReturn {
		t.Errorf("member outside: %#x, want return", s)
	}
	if s := gr.AlarmState(nil, inside); s != AlarmCalm {
		t.Errorf("all home: %#x, want calm", s)
	}
	for kind, want := range map[byte]Tactic{0x41: TacticAssist, 0x45: TacticUnengaged, 0x44: TacticStationary,
		0x4C: TacticDormant, 0x4E: TacticNearest, 0x54: TacticNearest, 0x48: TacticNearest} {
		if got := TacticOf(kind); got != want {
			t.Errorf("TacticOf(%#x) = %d, want %d", kind, got, want)
		}
	}
	if Dist(10, 10, 14, 11) != 4 || Dist(10, 10, 14, 12) != 5 || Dist(10, 10, 10, 13) != 3 {
		t.Error("Dist does not follow 0x00439944")
	}
	e := Entity{Flags4: 0x4C, WeaponMode: 2, HP: 0}
	if !Revives(&e) || e.HP != HPFull || e.WeaponMode != 1 {
		t.Errorf("kind 0x4C revive: %+v", e)
	}
	if e.WeaponMode = 0; Revives(&e) {
		t.Error("revived with no revives left")
	}
}
