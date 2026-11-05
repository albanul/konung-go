package resfmt

import (
	"encoding/binary"
	"fmt"
	"image"
	"strings"
)

// Text and fonts embedded in konung.exe's DGROUP section. Strings are
// zero-terminated CP866; the game draws the raw bytes with its own fonts, so
// they are kept as bytes and converted with CP866ToUTF8 only for display in
// tools and tests.

const (
	// Name tables, 32 bytes per entry (entity name routine VA 0x0042CA90).
	CreatureNamesVA = 0x0045E3BC // index = (entity +4 & 0x7F) - 0x41
	FirstNamesVA    = 0x0045E77C // index = entity +0xD4
	NicknamesVA     = 0x0045FCDC // index = entity +0xD5 (0 = none), appended after " "
	nameStride      = 32

	// Fonts (header + glyph pointer table, relocated at load, VA 0x0043AA7F).
	FontLargeVA = 0x00592CFC // character window title
	FontSmallVA = 0x005A31F4 // status line, dialogue text
	FontSlotVA  = 0x00593040 // save/load screen: slots other than the selected one (0x004315A8)
)

// CString returns the zero-terminated CP866 string at a DGROUP address.
func (t *ExeTables) CString(va int) []byte {
	if va < dgroupVA || va >= dgroupVA+dgroupSize {
		return nil
	}
	b := t.at(va)
	for i, c := range b {
		if c == 0 {
			return b[:i:i]
		}
	}
	return nil
}

// TableString returns entry i of a 32-byte string table.
func (t *ExeTables) TableString(tableVA, i int) []byte {
	if i < 0 {
		return nil
	}
	return t.CString(tableVA + i*nameStride)
}

// CP866ToUTF8 converts DOS Cyrillic text (the game's encoding).
func CP866ToUTF8(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		switch {
		case c < 0x80:
			sb.WriteByte(c)
		case c < 0xB0: // А..Я, а..п
			sb.WriteRune(rune(0x410 + int(c) - 0x80))
		case c >= 0xE0 && c < 0xF0: // р..я
			sb.WriteRune(rune(0x440 + int(c) - 0xE0))
		case c == 0xF0:
			sb.WriteRune('Ё')
		case c == 0xF1:
			sb.WriteRune('ё')
		default:
			sb.WriteRune('?')
		}
	}
	return sb.String()
}

// Font is one of the game's bitmap fonts. Header: byte first character,
// byte glyph count, int16 advance used for characters without a glyph;
// then count uint32 glyph addresses (0 = none; the last entry is unused). Each glyph is a Type 5
// RGB555 row-RLE frame; a character's advance is its glyph width (text
// width routine 0x0043F821).
type Font struct {
	First, Count int
	Missing      int
	Glyphs       []*image.RGBA // index = character - First; nil = no glyph
}

// Font decodes the font at va (FontLargeVA or FontSmallVA).
func (t *ExeTables) Font(va int) (*Font, error) {
	h := t.at(va)
	f := &Font{First: int(h[0]), Count: int(h[1]), Missing: int(int16(binary.LittleEndian.Uint16(h[2:])))}
	for i := 0; i < f.Count; i++ {
		p := int(binary.LittleEndian.Uint32(h[4+i*4:]))
		// The load-time relocation (0x0043AA7F) only fixes up count-1
		// pointers; the last entry is not a glyph.
		if p == 0 || i == f.Count-1 {
			f.Glyphs = append(f.Glyphs, nil)
			continue
		}
		if p < dgroupVA || p >= dgroupVA+dgroupSize {
			return nil, fmt.Errorf("font %#x glyph %d at %#x", va, i, p)
		}
		img, err := DecodeRLEFrame(t.at(p))
		if err != nil {
			return nil, fmt.Errorf("font %#x glyph %d: %w", va, i, err)
		}
		f.Glyphs = append(f.Glyphs, img)
	}
	return f, nil
}

// Glyph returns the glyph for CP866 character c, or nil.
func (f *Font) Glyph(c byte) *image.RGBA {
	if i := int(c) - f.First; i >= 0 && i < f.Count {
		return f.Glyphs[i]
	}
	return nil
}

// Advance is the horizontal step for c.
func (f *Font) Advance(c byte) int {
	if g := f.Glyph(c); g != nil {
		return g.Bounds().Dx()
	}
	return f.Missing
}

// Width is the pixel width of text (0x0043F821).
func (f *Font) Width(text []byte) int {
	w := 0
	for _, c := range text {
		w += f.Advance(c)
	}
	return w
}

// LineHeight is the line step of the wrapping text routine 0x0042CBBC.
const LineHeight = 15

// Wrap splits text into lines no wider than width like 0x0042CBBC: words
// keep their trailing space; a word that would reach width starts a new line.
func (f *Font) Wrap(text []byte, width int) [][]byte {
	var lines [][]byte
	var cur []byte
	curW := 0
	for len(text) > 0 {
		n := 0
		for n < len(text) && text[n] > ' ' {
			n++
		}
		if n < len(text) {
			n++ // the separating space (or control byte) stays with the word
		}
		word := text[:n]
		text = text[n:]
		w := f.Width(word)
		if curW+w >= width && len(cur) > 0 {
			lines = append(lines, cur)
			cur, curW = nil, 0
		}
		cur = append(cur, word...)
		curW += w
	}
	if len(cur) > 0 {
		lines = append(lines, cur)
	}
	return lines
}
