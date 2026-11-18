package main

import (
	"os"
	"strings"
	"testing"
)

// The example config is valid and sets every setting.
func TestExampleConfig(t *testing.T) {
	data, err := os.ReadFile("../../konung-server.conf.example")
	if err != nil {
		t.Fatal(err)
	}
	st := defaultSettings()
	st.Name, st.Port, st.MaxPlayers, st.Minutes, st.Speed, st.TeamDamage = "", 0, 0, 0, -1, false
	if err := st.applyConfig(data); err != nil {
		t.Fatal(err)
	}
	want := settings{Name: "Мой сервер", Port: 7777, MaxPlayers: 16, Minutes: 180, Speed: 0, TeamDamage: true}
	if st != want {
		t.Errorf("example = %+v, want %+v", st, want)
	}
	for key := range configKeys {
		if !strings.Contains(string(data), "\n"+key+" = ") {
			t.Errorf("the example does not show %q", key)
		}
	}
}

func TestConfig(t *testing.T) {
	st := defaultSettings()
	err := st.applyConfig([]byte("# comment\n\nname = Home\nspeed=2\nteam_damage = off\n"))
	if err != nil || st.Name != "Home" || st.Speed != 2 || st.TeamDamage || st.Minutes != 180 {
		t.Errorf("settings %+v, %v", st, err)
	}
	for _, bad := range []string{"nmae = x", "speed = 5", "team_damage = maybe", "port = http", "just text", "name ="} {
		s := defaultSettings()
		if err := s.applyConfig([]byte(bad)); err == nil {
			t.Errorf("%q accepted", bad)
		} else if !strings.Contains(err.Error(), "line 1") {
			t.Errorf("%q: %v (no line number)", bad, err)
		}
	}
}

// The config overrides the SERVER file; a missing default config is fine,
// a missing named one is not.
func TestLoadSettings(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/SERVER", []byte("TEAM 3 60\r\nHome Server\r\n"), 0o644)
	st, err := loadSettings(dir, "")
	if err != nil || st.Name != "Home Server" || st.Minutes != 60 {
		t.Fatalf("SERVER only: %+v, %v", st, err)
	}
	os.WriteFile(dir+"/server.conf", []byte("name = Config Name\n"), 0o644)
	if st, err = loadSettings(dir, ""); err != nil || st.Name != "Config Name" || st.Minutes != 60 {
		t.Errorf("with server.conf: %+v, %v", st, err)
	}
	if _, err := loadSettings(dir, dir+"/missing.conf"); err == nil {
		t.Error("a missing -config file was accepted")
	}
}
