package konungmenu

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	mapColumns       = 80
	mapRows          = 115
	mapPlanes        = 7
	mapStride        = 18400
	terrainTileCount = 304
)

func loadStartScene(graphPath, mapPath, objectsPath string) (*ebiten.Image, error) {
	graphData, err := os.ReadFile(graphPath)
	if err != nil {
		return nil, err
	}
	tiles, err := decodeGraphTiles(graphData)
	if err != nil {
		return nil, err
	}
	mapData, err := os.ReadFile(mapPath)
	if err != nil {
		return nil, err
	}
	if len(mapData) != mapStride*mapPlanes {
		return nil, fmt.Errorf("%s has unexpected map size %d", mapPath, len(mapData))
	}
	personData, err := os.ReadFile(objectsPath)
	if err != nil {
		return nil, err
	}
	person, err := decodeObjectFrame(personData, graphData, 4)
	if err != nil {
		return nil, err
	}

	scene := ebiten.NewImage(ScreenWidth, ScreenHeight)
	scene.Fill(color.RGBA{R: 14, G: 24, B: 17, A: 255})
	// KONUNG.1 is used as the provisional Old Camp map. The layer values are
	// drawn in file order; the original renderer's precise layer semantics are
	// still being traced.
	for plane := 0; plane < mapPlanes; plane++ {
		for cell := 0; cell < mapColumns*mapRows; cell++ {
			cx, cy := cell%mapColumns, cell/mapColumns
			x := ScreenWidth/2 + (cx-cy)*16 - 57
			y := 88 + (cx+cy-(mapColumns+mapRows)/2)*8 - 32
			// Map cells hold two byte-sized tile references. This interpretation
			// matches the byte range and the engine's 256-entry tile addressing.
			cellOffset := plane*mapStride + cell*2
			for _, id := range mapData[cellOffset : cellOffset+2] {
				if id == 0 || id == 0xff || int(id) >= len(tiles) || tiles[id] == nil {
					continue
				}
				drawAt(scene, tiles[id], x, y)
			}
		}
	}
	drawAt(scene, person, ScreenWidth/2-50, ScreenHeight/2-70)
	return scene, nil
}

func decodeGraphTiles(data []byte) ([]*ebiten.Image, error) {
	const graphBlock = 0x227e8
	if len(data) < graphBlock+8+512*8 {
		return nil, fmt.Errorf("GRAPH.RES is truncated before terrain index")
	}
	resourceSize := int(binary.LittleEndian.Uint32(data[graphBlock+4:]))
	resourceEnd := graphBlock + 8 + resourceSize
	if resourceEnd > len(data) || resourceEnd < graphBlock+8+512*8 {
		return nil, fmt.Errorf("GRAPH.RES terrain block has invalid size %d", resourceSize)
	}
	var palettes [256][256]color.RGBA
	for p := range palettes {
		for i := range palettes[p] {
			v := binary.LittleEndian.Uint16(data[(p*256+i)*2:])
			palettes[p][i] = color.RGBA{uint8(((v >> 10) & 31) * 255 / 31), uint8(((v >> 5) & 31) * 255 / 31), uint8((v & 31) * 255 / 31), 255}
			if v == 0x7c1f {
				palettes[p][i].A = 0
			}
		}
	}
	payloadStart := graphBlock + 8
	entries := data[payloadStart : payloadStart+512*8]
	blockStart := payloadStart + 512*8
	// GRAPH.RES has 304 terrain tiles in this block. Entry 304 is a larger
	// non-tile image (603x657), despite having a valid offset and palette.
	tiles := make([]*ebiten.Image, terrainTileCount)
	for i := 0; i < len(tiles); i++ {
		off := binary.LittleEndian.Uint32(entries[i*8:])
		palOff := binary.LittleEndian.Uint32(entries[i*8+4:])
		if off == 0xffffffff || palOff == 0xffffffff {
			break
		}
		end := uint32(resourceEnd - blockStart)
		if i+1 < 512 {
			next := binary.LittleEndian.Uint32(entries[(i+1)*8:])
			if next != 0xffffffff && next > off {
				end = next
			}
		}
		if end <= off || blockStart+int(end) > len(data) {
			return nil, fmt.Errorf("GRAPH.RES tile %d has invalid range", i)
		}
		palIndex := palOff / 512
		if palIndex >= 256 {
			return nil, fmt.Errorf("GRAPH.RES tile %d has invalid palette %d", i, palIndex)
		}
		img, err := decodeIndexedTile(data[blockStart+int(off):blockStart+int(end)], &palettes[palIndex])
		if err != nil {
			return nil, fmt.Errorf("decode GRAPH.RES tile %d: %w", i, err)
		}
		tiles[i] = ebiten.NewImageFromImage(img)
	}
	return tiles, nil
}

func decodeIndexedTile(src []byte, palette *[256]color.RGBA) (*image.RGBA, error) {
	if len(src) < 4 {
		return nil, fmt.Errorf("tile header truncated")
	}
	w, h := int(binary.LittleEndian.Uint16(src)), int(binary.LittleEndian.Uint16(src[2:]))
	if w < 1 || h < 1 || w > 512 || h > 512 || len(src) < 4+h*2 {
		return nil, fmt.Errorf("invalid tile dimensions %dx%d", w, h)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	pos := 4 + h*2
	for y := 0; y < h; y++ {
		rowSize := int(binary.LittleEndian.Uint16(src[4+y*2:]))
		end := pos + rowSize
		if end > len(src) {
			return nil, fmt.Errorf("tile row %d truncated", y)
		}
		x := 0
		for pos < end {
			n := int(src[pos])
			pos++
			if n == 0 {
				break
			}
			if n&0x80 != 0 {
				x += n & 0x7f
				continue
			}
			if pos+n > end {
				return nil, fmt.Errorf("tile row %d pixel run truncated", y)
			}
			for _, index := range src[pos : pos+n] {
				if x < w {
					img.SetRGBA(x, y, palette[index])
				}
				x++
			}
			pos += n
		}
		pos = end
	}
	return img, nil
}

func decodeObjectFrame(objects, graph []byte, objectIndex int) (*ebiten.Image, error) {
	const table = 12
	if objectIndex < 1 || objectIndex > 512 || len(objects) < table+512*8 {
		return nil, fmt.Errorf("OBJECTS.RES index table truncated")
	}
	i := (objectIndex - 1) * 8
	blockOffset := binary.LittleEndian.Uint32(objects[table+i+4:])
	base := int(0x1008 + blockOffset)
	if base < 0 || base+0x3228 > len(objects) {
		return nil, fmt.Errorf("OBJECTS.RES object %d is out of range", objectIndex)
	}
	if binary.BigEndian.Uint32(objects[base:]) != 2 {
		return nil, fmt.Errorf("OBJECTS.RES object %d is not a Type 2 animated object", objectIndex)
	}
	frameOffsets := make([]uint32, 1024)
	for i := range frameOffsets {
		frameOffsets[i] = binary.LittleEndian.Uint32(objects[base+0x728+i*4:])
	}
	for i := 0; i+1 < len(frameOffsets)-1; i += 2 {
		frameOffsets[i], frameOffsets[i+1] = frameOffsets[i+1], frameOffsets[i]
	}
	start := frameOffsets[0]
	end := frameOffsets[2]
	if end <= start || end == 0xffffffff {
		return nil, fmt.Errorf("OBJECTS.RES object %d has no first frame", objectIndex)
	}
	frameStart := base + 0x3228 + int(start)
	frameEnd := base + 0x3228 + int(end)
	if frameStart < 0 || frameEnd > len(objects) {
		return nil, fmt.Errorf("OBJECTS.RES object %d frame is out of range", objectIndex)
	}
	paletteIndex := 56
	if objectIndex <= 24 {
		paletteIndex += objectIndex - 1
	}
	var palette [256]color.RGBA
	for i := range palette {
		v := binary.LittleEndian.Uint16(graph[(paletteIndex*256+i)*2:])
		palette[i] = color.RGBA{uint8(((v >> 10) & 31) * 255 / 31), uint8(((v >> 5) & 31) * 255 / 31), uint8((v & 31) * 255 / 31), 255}
		if v == 0x7c1f {
			palette[i].A = 0
		}
	}
	img, err := decodeIndexedTile(objects[frameStart:frameEnd], &palette)
	if err != nil {
		return nil, fmt.Errorf("decode OBJECTS.RES person candidate: %w", err)
	}
	return ebiten.NewImageFromImage(img), nil
}
