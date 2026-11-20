package gamestate

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"konung1.5/internal/resfmt"
)

// The LAN misc item numbers map to the single-player items of the same
// name, and the exe's default amulet places are DefaultAmuletPoints.
func TestLANMiscTypesAndAmuletPoints(t *testing.T) {
	lan, err := os.ReadFile("../../assets/Konung/LAN_KONG.EXE")
	if err != nil {
		t.Skip("LAN_KONG.EXE not available:", err)
	}
	sp, err := os.ReadFile("../../assets/Konung/konung.exe")
	if err != nil {
		t.Skip("konung.exe not available:", err)
	}
	tables, err := resfmt.NewExeTables(sp)
	if err != nil {
		t.Fatal(err)
	}
	at := func(va int) []byte { return lan[va-lanDGroupVA+lanDGroupFile:] }
	for k, typ := range LANMiscTypes {
		name := at(0x0048222C + 64*k)
		name = name[:bytes.IndexByte(name, 0)]
		if got := tables.ItemName(resfmt.ItemTableMisc, typ); !bytes.Equal(got, name) {
			t.Errorf("LAN misc %d %q = single-player %d %q", k, resfmt.CP866ToUTF8(name), typ, resfmt.CP866ToUTF8(got))
		}
	}
	for k, p := range DefaultAmuletPoints {
		for i := range 3 {
			if got := int(int32(binary.LittleEndian.Uint32(at(0x004826CC + 12*k + 4*i)))); got != p[i] {
				t.Errorf("point %d[%d] = %d, exe %d", k, i, p[i], got)
			}
		}
	}
	for k := 1; k <= 7; k++ {
		if AmuletKind(AmuletKindType(k)) != k {
			t.Errorf("kind %d: type %d", k, AmuletKindType(k))
		}
	}
	if AmuletKind(MiscWords) != 0 || AmuletKind(MiscApple) != 0 {
		t.Error("not amulet items have a kind")
	}
	if !LANKeepsOnLoad(true, CatQuest, MiscWine) || LANKeepsOnLoad(true, CatQuest, MiscAmuletLR) {
		t.Error("load filter: wine dropped or a joined amulet kept")
	}
}
