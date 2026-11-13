package gamestate

import (
	"bytes"
	"os"
	"testing"
)

func TestLANConfigRoundTrip(t *testing.T) {
	c := LANConfig{Resolution: 1, SavePassword: true, Settings: [6]uint32{1, 2, 3, 4, 5, 6},
		Name: []byte("Volk"), Password: []byte("secret7"), Clan: []byte{0x90, 0xe3, 0xe1, 0xe1, 0xaa, 0xa8, 0xa5},
		Address: []byte("192.168.0.5")}
	data := c.Marshal()
	if len(data) != LANConfigSize || LANConfigSize != 486 {
		t.Fatalf("size %d", len(data))
	}
	got := ParseLANConfig(data)
	if got.Resolution != 1 || !got.SavePassword || got.Settings != c.Settings || string(got.Name) != "Volk" ||
		string(got.Password) != "secret7" || !bytes.Equal(got.Clan, c.Clan) || string(got.Address) != "192.168.0.5" {
		t.Errorf("round trip = %+v", got)
	}
	// Without «Сохранить пароль» the password is not written (0x004329AE).
	c.SavePassword = false
	data = c.Marshal()
	if !bytes.Equal(data[0x3D:0x46], make([]byte, 9)) {
		t.Error("password stored without SavePassword")
	}
	if p := ParseLANConfig(data).Password; p != nil {
		t.Errorf("password %q read back", p)
	}
	if c := ParseLANConfig(nil); c.Name != nil || c.SavePassword {
		t.Errorf("missing file = %+v", c)
	}
}

func TestParseServerFile(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/SERVER")
	if err != nil {
		t.Skip("SERVER not available:", err)
	}
	f, err := ParseServerFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if f.Mode != "TEAM" || f.Teams != 3 || f.Minutes != 180 || string(f.Name) != "Home Server" || len(f.Points) != 6 || f.Points[0] != [3]int{5, 64, 32} {
		t.Errorf("SERVER = %+v", f)
	}
}

func TestDefaultServerName(t *testing.T) {
	data, err := os.ReadFile("../../assets/Konung/LAN_KONG.EXE")
	if err != nil {
		t.Skip("LAN_KONG.EXE not available:", err)
	}
	const off = 0x470719 - 0x470000 + 0x54600
	if !bytes.Equal(data[off:off+len(DefaultServerName)+1], append(append([]byte(nil), DefaultServerName...), 0)) {
		t.Error("DefaultServerName differs from the exe string at 0x00470719")
	}
}
