package gamestate

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

// The table: players by rating (÷ 10), clans by their members' mean,
// every row ranked; own outside the top ten takes row 9, its clan too.
func TestLANRatingTable(t *testing.T) {
	clans := []string{"Византийцы", "Русские", "Викинги", "Варяги"}
	var accounts []LANRatingAccount
	for k := range 12 {
		accounts = append(accounts, LANRatingAccount{Name: fmt.Sprintf("P%d", k), Clan: 1, Rating: 1000 + 10*k})
	}
	accounts[0].Clan, accounts[0].Rating = 0, 5000 // Византийцы: the best clan
	accounts[3].Clan, accounts[3].Rating = 3, 1    // Варяги: the worst
	accounts[5].Amulets, accounts[5].KillsHeroes = 2, 3
	rows := LANRatingTable(accounts, clans, "P1")
	if rows[0].Name != "P0" || rows[0].Rank != 1 || rows[0].Rating != 500 || rows[0].Clan != "Византийцы" {
		t.Errorf("row 0 = %+v", rows[0])
	}
	if rows[1].Name != "P11" || rows[1].Rating != 111 {
		t.Errorf("row 1 = %+v", rows[1])
	}
	// The clans: Византийцы 5000, Русские mean of the ten at 1010…1110
	// except P3 (P1, P2, P4…P11), Викинги none (0), Варяги 1.
	want := []string{"Византийцы", "Русские", "Варяги", "Викинги"}
	for k, c := range want {
		if rows[k].ClanName != c || rows[k].ClanRank != k+1 {
			t.Errorf("clan row %d = %q rank %d, want %q", k, rows[k].ClanName, rows[k].ClanRank, c)
		}
	}
	if rows[4].ClanName != "" || rows[4].ClanRank != 0 || rows[8].Rank != 9 {
		t.Errorf("rows past the clans: %+v, %+v", rows[4], rows[8])
	}
	// P1 (1010) is 11th: row 9 is P1 with rank 11; P3 (rating 1) is last.
	if rows[9].Name != "P1" || rows[9].Rank != 11 || rows[9].Rating != 101 {
		t.Errorf("own row = %+v", rows[9])
	}
	// P0, P11, P10, P9, P8, P7, P6, P5, P4, P2 | P1, P3: P5 is 8th, row
	// 9 stays the 10th.
	if r := LANRatingTable(accounts, clans, "P5"); r[9].Name != "P2" || r[9].Rank != 10 || r[7].Name != "P5" || r[7].Amulets != 2 || r[7].KillsHeroes != 3 {
		t.Errorf("P5 in the top ten: row 9 %+v, row 7 %+v", r[9], r[7])
	}
}

// HISCORES.DAT: the installed file reads and writes back byte for byte.
func TestLANHiscores(t *testing.T) {
	b, err := os.ReadFile("../../assets/Konung/HISCORES.DAT")
	if err != nil {
		t.Skip("HISCORES.DAT missing:", err)
	}
	rows := ParseLANHiscores(b)
	if len(rows) != LANRatingRows {
		t.Fatalf("%d rows", len(rows))
	}
	for k, r := range rows {
		if r.Rank != k+1 {
			t.Errorf("row %d rank %d", k, r.Rank)
		}
	}
	if got := EncodeLANHiscores(rows); !bytes.Equal(got, b) {
		t.Error("the rows do not write back as the file")
	}
}

// Own clan outside the top ten takes row 9's clan, with its rank.
func TestLANRatingOwnClan(t *testing.T) {
	var clans []string
	var accounts []LANRatingAccount
	for c := range 12 {
		clans = append(clans, fmt.Sprintf("C%d", c))
		accounts = append(accounts, LANRatingAccount{Name: fmt.Sprintf("P%d", c), Clan: c, Rating: 2000 - 100*c})
	}
	rows := LANRatingTable(accounts, clans, "P11")
	if rows[9].ClanName != "C11" || rows[9].ClanRank != 12 || rows[9].ClanRating != 90 || rows[8].ClanName != "C8" {
		t.Errorf("row 9 clan = %q rank %d rating %d; row 8 %q", rows[9].ClanName, rows[9].ClanRank, rows[9].ClanRating, rows[8].ClanName)
	}
	if rows := LANRatingTable(accounts, clans, "P3"); rows[9].ClanName != "C9" {
		t.Errorf("own clan in the top ten: row 9 clan %q", rows[9].ClanName)
	}
}
