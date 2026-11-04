package gamestate

import (
	"testing"

	"konung1.5/internal/resfmt"
)

func TestHoveredItemAndTooltips(t *testing.T) {
	e := Entity{Equipment: [6]int{1, 2, 3, 4, 5, 6}, Extra: [5]int{7, 8, 9, 10, 11}, Quiver: 12, WeaponMode: WeaponAxe}
	want := map[int]int{8: 5, 9: 1, 10: 6, 11: 4, 12: 2, 13: 12, 14: 7, 18: 11, 19: 0, 30: 0}
	for spot, item := range want {
		if got := HoveredItem(e, spot); got != item {
			t.Errorf("hotspot %d: item %d, want %d", spot, got, item)
		}
	}
	e.WeaponMode = WeaponBlade
	if HoveredItem(e, 9) != 3 {
		t.Error("blade mode: hand box shows the blade")
	}
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if got := resfmt.CP866ToUTF8(tb.CString(ButtonTooltips[6])); got != "Панель персонажа (I)" {
		t.Errorf("tooltip 7 = %q", got)
	}
}

func TestItemDescriptionGame0(t *testing.T) {
	g, err := Parse(load(t, "GAME.0"))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := resfmt.NewExeTables(load(t, "konung.exe"))
	if err != nil {
		t.Fatal(err)
	}
	for item, want := range map[int]string{
		1:    "Нож, убойная сила 60, износ 16/16, вес 0.20, требует:  Сил:3",
		2:    "Левая часть амулета Дракона, вес 0.10",
		3:    "Лечебный бальзам, концентрация  7.00, вес 0.50",
		482:  "Дубина, убойная сила 80, износ 5/5, вес 1.20, требует:  Вын:10",
		483:  "Доспех кожаный, броня: 24, износ 12/12, вес 2.70, требует:  Вын:12",
		1011: "Колчан стрел: 30 медные стрелы, убойная сила 2, вес 1.20",
		1015: "Ожерелье, вес 0.20 Вын+1",
		1109: "Палица, убойная сила 284, износ 21/29, вес 2.00 Сил+6, требует:  Вын:65",
		1028: "Составной лук, убойная сила 120, износ 51/87, вес 1.30 Сил+5, требует:  Лов:74",
		126:  "Кинжал, убойная сила 84, износ 110/110, вес 0.30, на червяков, требует:  Сил:21",
		127:  "Шлем гребневый, броня: 48, износ 36/36, вес 1.10 Здр+80, требует:  Здр:90, требует:  Лов:56",
		133:  "Богатырские доспехи, броня: 200, износ 110/110, вес 5.00 Лов=100 Вын-20, требует:  Хар:35, требует:  Вын:95",
		1352: "Колчан стрел: 30 серебряные стрелы, убойная сила 4, вес 1.80, на оборотня",
		1398: "Земляной орех, вес 1.00",
	} {
		if got := resfmt.CP866ToUTF8(g.ItemDescription(tb, item)); got != want {
			t.Errorf("item %d:\n got %q\nwant %q", item, got, want)
		}
	}
}
