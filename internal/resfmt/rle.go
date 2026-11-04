package resfmt

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
)

// DecodeRLEFrame decodes a Konung Type 5 RLE-compressed sprite frame into an RGBA image.
// Format:
//
//	Width:       uint16 (Little-Endian)
//	Height:      uint16 (Little-Endian)
//	RowSizes:    [Height]uint16 (Little-Endian) - byte size of each compressed row
//	RowData:     Byte stream of RLE commands per row
//	  cmd == 0:  End of row
//	  cmd < 0:   Transparent run: skip (cmd & 0x7F) pixels
//	  cmd > 0:   Opaque run: read `cmd` 16-bit RGB 555 pixels (0RRRRRGGGGGBBBBB)
func DecodeRLEFrame(frameData []byte) (*image.RGBA, error) {
	if len(frameData) < 4 {
		return nil, fmt.Errorf("frame data too small for header: %d bytes", len(frameData))
	}

	width := int(binary.LittleEndian.Uint16(frameData[0:2]))
	height := int(binary.LittleEndian.Uint16(frameData[2:4]))

	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid frame dimensions: %dx%d", width, height)
	}

	tableOffset := 4
	tableSize := height * 2
	if len(frameData) < tableOffset+tableSize {
		return nil, fmt.Errorf("frame data smaller than row size table (%d < %d)", len(frameData), tableOffset+tableSize)
	}

	rowSizes := make([]int, height)
	for i := 0; i < height; i++ {
		rowSizes[i] = int(binary.LittleEndian.Uint16(frameData[tableOffset+i*2 : tableOffset+i*2+2]))
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	dataOffset := tableOffset + tableSize
	for y := 0; y < height; y++ {
		sz := rowSizes[y]
		if dataOffset+sz > len(frameData) {
			return nil, fmt.Errorf("row %d exceeds frame data (offset %d, size %d, total %d)",
				y, dataOffset, sz, len(frameData))
		}

		rowData := frameData[dataOffset : dataOffset+sz]
		dataOffset += sz

		x := 0
		idx := 0
		for idx < len(rowData) {
			cmd := int8(rowData[idx])
			idx++

			if cmd == 0 {
				// End of row
				break
			} else if cmd < 0 {
				// Transparent run: lower 7 bits
				skip := int(cmd & 0x7F)
				x += skip
			} else {
				// Opaque run: `count` RGB 555 pixels
				count := int(cmd)
				for i := 0; i < count; i++ {
					if idx+2 > len(rowData) {
						break
					}
					val := binary.LittleEndian.Uint16(rowData[idx : idx+2])
					idx += 2

					// 15-bit RGB 555 (0_RRRRR_GGGGG_BBBBB):
					// Red:   bits 14..10 (5 bits) -> [0..255]
					// Green: bits  9..5  (5 bits) -> [0..255]
					// Blue:  bits  4..0  (5 bits) -> [0..255]
					r := uint8(((val >> 10) & 0x1F) * 255 / 31)
					g := uint8(((val >> 5) & 0x1F) * 255 / 31)
					b := uint8((val & 0x1F) * 255 / 31)

					if x < width {
						img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
					}
					x++
				}
			}
		}
	}

	return img, nil
}

// LoadType5Frames reads all frames from an open Type 5 .RES file.
func LoadType5Frames(r io.Reader) ([]*image.RGBA, error) {
	var frames []*image.RGBA
	frameIndex := 0

	for {
		var header struct {
			Tag  uint32 // 0x00000005 (Big-Endian in file)
			Size uint32 // Frame data size (Little-Endian)
		}

		if err := binary.Read(r, binary.BigEndian, &header.Tag); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return frames, fmt.Errorf("error reading tag for frame %d: %w", frameIndex, err)
		}

		if err := binary.Read(r, binary.LittleEndian, &header.Size); err != nil {
			return frames, fmt.Errorf("error reading size for frame %d: %w", frameIndex, err)
		}

		if header.Tag != 5 {
			return frames, fmt.Errorf("frame %d: unexpected tag 0x%X (expected 5)", frameIndex, header.Tag)
		}

		frameData := make([]byte, header.Size)
		if _, err := io.ReadFull(r, frameData); err != nil {
			return frames, fmt.Errorf("error reading frame data %d (%d bytes): %w", frameIndex, header.Size, err)
		}

		img, err := DecodeRLEFrame(frameData)
		if err != nil {
			return frames, fmt.Errorf("error decoding frame %d: %w", frameIndex, err)
		}

		frames = append(frames, img)
		frameIndex++
	}

	return frames, nil
}

// LoadType5File opens a Type 5 .RES file from disk and decodes all frames.
func LoadType5File(filePath string) ([]*image.RGBA, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return LoadType5Frames(f)
}
