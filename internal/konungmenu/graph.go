package konungmenu

import (
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"os"

	"konung1.5/internal/resource"
)

// loadGraphSprite reads a Type 5 image after GRAPH.RES's 256x256 RGB555 palettes.
// Sprite 4 is the plain sword pointer; nearby sprites add contextual markers.
func loadGraphSprite(graphPath string, spriteIndex int) (image.Image, error) {
	f, err := os.Open(graphPath)
	if err != nil {
		return nil, fmt.Errorf("open graph resource %q: %w", graphPath, err)
	}
	defer f.Close()

	const paletteBytes = 256 * 256 * 2
	if _, err := f.Seek(paletteBytes, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek past GRAPH.RES palettes: %w", err)
	}
	for i := 0; ; i++ {
		var header [8]byte
		if _, err := io.ReadFull(f, header[:]); err != nil {
			return nil, fmt.Errorf("read GRAPH.RES sprite %d header: %w", i, err)
		}
		if tag := binary.BigEndian.Uint32(header[:4]); tag != 5 {
			return nil, fmt.Errorf("GRAPH.RES sprite %d has tag %d, want 5", i, tag)
		}
		size := binary.LittleEndian.Uint32(header[4:])
		if size < 4 || size > 16*1024*1024 {
			return nil, fmt.Errorf("GRAPH.RES sprite %d has invalid frame size %d", i, size)
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(f, data); err != nil {
			return nil, fmt.Errorf("read GRAPH.RES sprite %d: %w", i, err)
		}
		if i == spriteIndex {
			img, err := resource.DecodeRLEFrame(data)
			if err != nil {
				return nil, fmt.Errorf("decode GRAPH.RES sprite %d: %w", i, err)
			}
			return img, nil
		}
	}
}
