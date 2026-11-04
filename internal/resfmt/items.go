package resfmt

import "encoding/binary"

// Item type tables in konung.exe. An item record's category (byte +0)
// selects the table and its type (byte +3) the record, as the icon lookup
// 0x0042C870 does. Every record starts with its CP866 name.
const (
	ItemTableGear  = iota // categories 0–5 and 12: weapons, armour, ammunition (64-byte records, 0x00459BB0)
	ItemTableJewel        // categories 6–8: necklaces, bracelets, rings (0x00458110)
	ItemTableHerb         // category 9: herbs and potions (0x004594B0)
	ItemTableMisc         // other categories: quest items, books, maps (0x0046053C)
)

var itemTables = [4]struct{ va, stride, icon int }{
	{0x00459BB0, 64, 0x3A},
	{0x00458110, 128, 0x7C},
	{0x004594B0, 128, 0x7C},
	{0x0046053C, 128, 0x7C},
}

// ItemTableOf returns the type table an item category uses (0x0042C870).
func ItemTableOf(category int) int {
	switch {
	case category <= 5 || category == 12:
		return ItemTableGear
	case category <= 8:
		return ItemTableJewel
	case category == 9:
		return ItemTableHerb
	}
	return ItemTableMisc
}

func (t *ExeTables) itemRecord(table, typ int) int {
	if table < 0 || table >= len(itemTables) || typ < 0 || typ > 255 {
		return -1
	}
	it := itemTables[table]
	return it.va + typ*it.stride
}

// ItemName returns the CP866 name of item type typ in a type table.
func (t *ExeTables) ItemName(table, typ int) []byte {
	va := t.itemRecord(table, typ)
	if va < 0 {
		return nil
	}
	return t.CString(va)
}

// ItemIcon returns the INTR slot of an item type's icon (the int16 at
// record +0x3A for gear, +0x7C otherwise), or -1.
func (t *ExeTables) ItemIcon(table, typ int) int {
	va := t.itemRecord(table, typ)
	if va < 0 {
		return -1
	}
	n := int(int16(binary.LittleEndian.Uint16(t.at(va + itemTables[table].icon))))
	if n < 0 || n >= 512 {
		return -1
	}
	return n
}

// ItemGroundImage returns the INTR slot an item type is drawn with when it
// lies alone on the ground (0x00439DF0): the word after the inventory icon
// (gear +0x3C, other tables +0x7E), or -1.
func (t *ExeTables) ItemGroundImage(table, typ int) int {
	va := t.itemRecord(table, typ)
	if va < 0 {
		return -1
	}
	n := int(int16(binary.LittleEndian.Uint16(t.at(va + itemTables[table].icon + 2))))
	if n < 0 || n >= 512 {
		return -1
	}
	return n
}

// ItemWeight returns an item type's weight in grams: gear record +0x38,
// other tables +0x7A (0x0041B898).
func (t *ExeTables) ItemWeight(table, typ int) int {
	va := t.itemRecord(table, typ)
	if va < 0 {
		return 0
	}
	off := 0x7A
	if table == ItemTableGear {
		off = 0x38
	}
	return int(int16(binary.LittleEndian.Uint16(t.at(va + off))))
}

// GearValue returns a gear type's damage (weapons) or armour value, the
// uint16 at record +0x2C (0x0041AC1C, 0x0041AF48).
func (t *ExeTables) GearValue(typ int) int {
	va := t.itemRecord(ItemTableGear, typ)
	if va < 0 {
		return 0
	}
	return int(binary.LittleEndian.Uint16(t.at(va + 0x2C)))
}

// GearRequirement returns the stat id (StatGetter numbering) and minimum a
// gear type requires (record +0x30, +0x32; minimum 0 = none, 0x00418364).
func (t *ExeTables) GearRequirement(typ int) (stat, min int) {
	va := t.itemRecord(ItemTableGear, typ)
	if va < 0 {
		return 0, 0
	}
	return int(int16(binary.LittleEndian.Uint16(t.at(va + 0x30)))), int(int16(binary.LittleEndian.Uint16(t.at(va + 0x32))))
}

// Enchant is an entry of the enchantment table 0x00458590 (16 bytes), indexed
// by item byte +1 or entity bytes +0xD2/+0xD3.
type Enchant struct {
	ReqStat, ReqMin int // +2, +3: required stat (StatGetter id) and minimum; ReqMin 0 = none
	Effects         [3]EnchantEffect
}

// EnchantEffect is one {stat, mask, value} triple (+4, +8, +0xC). Stat is
// 1-based into the bonus array of 0x0041C750 (0 = none); mask 1 stacks the
// value, mask 0 replaces the stat with it.
type EnchantEffect struct{ Stat, Mask, Value int }

const enchantTableVA = 0x00458590

// Enchant returns enchantment i (i ≥ 0).
func (t *ExeTables) Enchant(i int) Enchant {
	b := t.at(enchantTableVA + i*16)
	e := Enchant{ReqStat: int(b[2]), ReqMin: int(b[3])}
	for k := range e.Effects {
		r := b[4+k*4:]
		e.Effects[k] = EnchantEffect{Stat: int(r[0]), Mask: int(r[1]), Value: int(int16(binary.LittleEndian.Uint16(r[2:])))}
	}
	return e
}

// ItemSpecial returns entry i of the special-property table (records at
// 0x0046238C + i·64: name +0, value +0x2C, kind +0x2E; read here at 0x004623B8)
// (64 bytes; item byte +2): a damage value and the target kind it applies
// to (0x0041AF48 compares the kind with its argument).
func (t *ExeTables) ItemSpecial(i int) (value, kind int) {
	b := t.at(0x004623B8 + i*64)
	return int(binary.LittleEndian.Uint16(b)), int(int16(binary.LittleEndian.Uint16(b[2:])))
}

// ItemField16 returns the int16 at offset off of an item type's record.
func (t *ExeTables) ItemField16(table, typ, off int) int {
	va := t.itemRecord(table, typ)
	if va < 0 {
		return 0
	}
	return int(int16(binary.LittleEndian.Uint16(t.at(va + off))))
}

// StatName returns the short stat name for a requirement/enchantment stat id
// (1-based, table 0x00465A24, 5 bytes each, with a leading space): " Хар",
// " Лов", " Хар", " Здр", " Сил", " Вын", " Брн", " Удр", " Вер".
func (t *ExeTables) StatName(id int) []byte {
	if id < 1 || id > 9 {
		return nil
	}
	return t.CString(0x00465A24 + (id-1)*5)
}

// ItemSpecialName returns the text of special property i (record start
// 0x0046238C + i·64; ItemSpecial reads its +0x2C/+0x2E), e.g. ", на червяков".
func (t *ExeTables) ItemSpecialName(i int) []byte { return t.CString(0x0046238C + i*64) }
