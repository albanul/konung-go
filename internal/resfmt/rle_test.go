package resfmt

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"os"
	"testing"
)

func TestDecodeRLEFrame_EmptyData(t *testing.T) {
	_, err := DecodeRLEFrame(nil)
	if err == nil {
		t.Fatal("expected error for nil data, got nil")
	}

	_, err = DecodeRLEFrame([]byte{1, 2, 3})
	if err == nil {
		t.Fatal("expected error for data < 4 bytes, got nil")
	}
}

func TestDecodeRLEFrame_Synthetic(t *testing.T) {
	// Build a 4x2 frame:
	// Row 0: skip 1, 2 pixels (pure red 0x7C00, pure blue 0x001F), end (0)
	// Row 1: 3 pixels (pure green 0x03E0 x 3), skip 1, end (0)

	var buf bytes.Buffer
	// Width = 4, Height = 2
	binary.Write(&buf, binary.LittleEndian, uint16(4))
	binary.Write(&buf, binary.LittleEndian, uint16(2))

	// Row 0 data
	var row0 bytes.Buffer
	row0.WriteByte(0x81)                                     // skip 1 (cmd = -1)
	row0.WriteByte(2)                                        // 2 pixels
	binary.Write(&row0, binary.LittleEndian, uint16(0x7C00)) // Red (15-bit: R=31, G=0, B=0)
	binary.Write(&row0, binary.LittleEndian, uint16(0x001F)) // Blue (15-bit: R=0, G=0, B=31)
	row0.WriteByte(0)                                        // End of row

	// Row 1 data
	var row1 bytes.Buffer
	row1.WriteByte(3)                                        // 3 pixels
	binary.Write(&row1, binary.LittleEndian, uint16(0x03E0)) // Green (15-bit: R=0, G=31, B=0)
	binary.Write(&row1, binary.LittleEndian, uint16(0x03E0))
	binary.Write(&row1, binary.LittleEndian, uint16(0x03E0))
	row1.WriteByte(0x81) // skip 1 (cmd = -1)
	row1.WriteByte(0)    // End of row

	// Row sizes table
	binary.Write(&buf, binary.LittleEndian, uint16(row0.Len()))
	binary.Write(&buf, binary.LittleEndian, uint16(row1.Len()))

	// Payload
	buf.Write(row0.Bytes())
	buf.Write(row1.Bytes())

	img, err := DecodeRLEFrame(buf.Bytes())
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 2 {
		t.Fatalf("expected dimensions 4x2, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}

	// Verify Row 0
	// Pixel (0, 0) is transparent
	if p := img.At(0, 0).(color.RGBA); p.A != 0 {
		t.Errorf("pixel (0,0) expected transparent, got %v", p)
	}
	// Pixel (1, 0) is Red
	if p := img.At(1, 0).(color.RGBA); p.R != 255 || p.G != 0 || p.B != 0 || p.A != 255 {
		t.Errorf("pixel (1,0) expected red (255,0,0,255), got %v", p)
	}
	// Pixel (2, 0) is Blue
	if p := img.At(2, 0).(color.RGBA); p.R != 0 || p.G != 0 || p.B != 255 || p.A != 255 {
		t.Errorf("pixel (2,0) expected blue (0,0,255,255), got %v", p)
	}
	// Pixel (3, 0) is transparent
	if p := img.At(3, 0).(color.RGBA); p.A != 0 {
		t.Errorf("pixel (3,0) expected transparent, got %v", p)
	}

	// Verify Row 1
	for x := 0; x < 3; x++ {
		p := img.At(x, 1).(color.RGBA)
		if p.R != 0 || p.G != 255 || p.B != 0 || p.A != 255 {
			t.Errorf("pixel (%d,1) expected green (0,255,0,255), got %v", x, p)
		}
	}
	if p := img.At(3, 1).(color.RGBA); p.A != 0 {
		t.Errorf("pixel (3,1) expected transparent, got %v", p)
	}
}

func TestLoadType5File_RealFile(t *testing.T) {
	const resPath = "../../assets/Konung/MENU640R.RES"
	if _, err := os.Stat(resPath); os.IsNotExist(err) {
		t.Skip("skipping real file test: MENU640R.RES not found")
	}

	frames, err := LoadType5File(resPath)
	if err != nil {
		t.Fatalf("failed to load real Type 5 file %s: %v", resPath, err)
	}

	if len(frames) != 15 {
		t.Errorf("expected 15 frames in MENU640R.RES, got %d", len(frames))
	}

	// Check frame 0 is 640x480
	if frames[0].Bounds().Dx() != 640 || frames[0].Bounds().Dy() != 480 {
		t.Errorf("expected frame 0 to be 640x480, got %dx%d", frames[0].Bounds().Dx(), frames[0].Bounds().Dy())
	}
}
