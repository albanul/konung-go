package gamestate

import "testing"

func TestLeaveMapRemovesDeadElder(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	st := g.settlementOn(1)
	if st == nil {
		t.Fatal("no settlement on KONUNG.1")
	}
	elder := st.Posts[0].Entity
	gr := g.Groups[st.Group]
	if elder < gr.First || elder >= gr.First+gr.Count {
		t.Fatalf("elder %d outside group %d (%d+%d)", elder, st.Group, gr.First, gr.Count)
	}
	next := g.Entities[elder+1]
	bag := g.Entities[elder].Bag[0]
	posts := st.Posts
	g.Entities[elder].Flags4 |= 0x80
	g.LeaveMap(1)
	gr2 := g.Groups[st.Group]
	if gr2.Count != gr.Count-1 {
		t.Fatalf("group count %d, want %d", gr2.Count, gr.Count-1)
	}
	if moved := g.Entities[elder]; moved.Index != elder || moved.NameIndex != next.NameIndex || moved.Row != next.Row {
		t.Errorf("slot %d holds %+v, want the next member moved down", elder, moved)
	}
	if bag != 0 && g.items[bag*itemStride] != 0xFF {
		t.Errorf("the dead elder's bag item %d was not freed", bag)
	}
	for k := 1; k < len(posts); k++ {
		want := posts[k].Entity
		if want > elder {
			want--
		}
		if st.Posts[k].Entity != want {
			t.Errorf("post %d entity %d, want %d", k, st.Posts[k].Entity, want)
		}
	}
	e0 := st.Posts[0].Entity
	if e0 == 0 || g.Entities[e0].Model >= 3 || g.Entities[e0].Dialogue != st.Posts[0].Dialogue {
		t.Errorf("post 0 = %d (model %d, dialogue %d), want a new elder with dialogue %d",
			e0, g.Entities[e0].Model, g.Entities[e0].Dialogue, st.Posts[0].Dialogue)
	}
}
