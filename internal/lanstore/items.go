package lanstore

import (
	"database/sql"
	"fmt"
)

// Item slots of a SAVITEMS.DAT record (0x200 bytes per account; written by
// 0x00444918, read by 0x0044444C): 6 equipment slots and 5 jewellery boxes,
// the 32-cell bag, and the boxed arrows.
const (
	SlotEquip  = "equip"  // 6 × 7 bytes at +0x00
	SlotJewel  = "jewel"  // 5 × 3 bytes at +0x2C (no condition)
	SlotBag    = "bag"    // 32 × 7 bytes at +0x3B
	SlotArrows = "arrows" // type +0x2A, count +0x2B

	EquipSlots = 6
	JewelSlots = 5
	BagSlots   = 32
)

// Item is one saved item: the GAME.N item fields the record keeps
// (category +0, enchantment +1, type +3, condition +4, maximum +8; for
// arrows the count).
type Item struct {
	Slot         string
	Index        int
	Category     int
	Enchantment  int
	Type         int
	Flag         bool // 0x80 of the stored category (identified by this player, a guess)
	Condition    float64
	MaxCondition float64
	Count        int
}

// keepOnLoad is the load filter of 0x0044444C: bag items of these kinds do
// not come back into a new session (amulet parts — category 11, type ≥ 10 —
// start every session anew).
func keepOnLoad(it Item) bool {
	if it.Slot != SlotBag {
		return true
	}
	switch {
	case it.Category > 12,
		it.Category <= 5 && it.Type >= 70,
		it.Category == 9 && it.Type >= 14,
		it.Category == 11 && it.Type >= 10:
		return false
	}
	return true
}

func validSlot(it Item) error {
	n := map[string]int{SlotEquip: EquipSlots, SlotJewel: JewelSlots, SlotBag: BagSlots, SlotArrows: 1}[it.Slot]
	if n == 0 || it.Index < 0 || it.Index >= n {
		return fmt.Errorf("lanstore: item slot %s %d", it.Slot, it.Index)
	}
	return nil
}

// SaveItems replaces an account's saved items (logout, quit, the session's
// end).
func (s *Store) SaveItems(name string, items []Item) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRow(`SELECT id FROM accounts WHERE name = ?`, name).Scan(&id); err != nil {
		return fmt.Errorf("lanstore: account %q: %w", name, err)
	}
	if err := replaceItems(tx, id, items); err != nil {
		return err
	}
	return tx.Commit()
}

// replaceItems replaces account id's saved items.
func replaceItems(tx *sql.Tx, id int64, items []Item) error {
	if _, err := tx.Exec(`DELETE FROM items WHERE account_id = ?`, id); err != nil {
		return err
	}
	for _, it := range items {
		if err := validSlot(it); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO items (account_id, slot, idx, category, enchantment, type, flag, condition, max_condition, count)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, it.Slot, it.Index, it.Category, it.Enchantment, it.Type, it.Flag, it.Condition, it.MaxCondition, it.Count); err != nil {
			return err
		}
	}
	return nil
}

// LoadItems returns an account's items for entering the game, without the
// kinds the original drops on load.
func (s *Store) LoadItems(name string) ([]Item, error) {
	rows, err := s.db.Query(`SELECT i.slot, i.idx, i.category, i.enchantment, i.type, i.flag, i.condition, i.max_condition, i.count
		FROM items i JOIN accounts a ON a.id = i.account_id WHERE a.name = ? ORDER BY i.slot, i.idx`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.Slot, &it.Index, &it.Category, &it.Enchantment, &it.Type, &it.Flag,
			&it.Condition, &it.MaxCondition, &it.Count); err != nil {
			return nil, err
		}
		if keepOnLoad(it) {
			out = append(out, it)
		}
	}
	return out, rows.Err()
}
