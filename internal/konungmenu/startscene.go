package konungmenu

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/resource"
)

const (
	mapColumns       = 80
	mapRows          = 115
	mapPlanes        = 7
	mapStride        = 18400
	terrainTileCount = 304
	grassMapPlane    = 1
	grassFillTile    = 15
	fighterObjectID  = 15
	fighterSwordID   = 10
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
	fighter, err := decodeObjectFrame(personData, graphData, fighterObjectID)
	if err != nil {
		return nil, err
	}
	swordFrame, err := loadGraphSprite(graphPath, fighterSwordID)
	if err != nil {
		return nil, err
	}
	sword := ebiten.NewImageFromImage(swordFrame)

	scene := ebiten.NewImage(ScreenWidth, ScreenHeight)
	// Fill the viewport with the game's grass tile so map transparency reads as
	// open ground instead of the dark outside-map color.
	grass := tiles[grassFillTile]
	for y := -64; y < ScreenHeight+64; y += 32 {
		for x := -114; x < ScreenWidth+114; x += 56 {
			drawAt(scene, grass, x, y)
		}
	}
	type tilePlacement struct {
		image *ebiten.Image
		x, y  int
	}
	var placements []tilePlacement
	minX, minY := int(^uint(0)>>1), int(^uint(0)>>1)
	maxX, maxY := -minX, -minY
	// KONUNG.1 is still a provisional Old Camp candidate. Render its grass
	// plane only; the other planes' meaning and draw order are not yet verified.
	for cell := 0; cell < mapColumns*mapRows; cell++ {
		cx, cy := cell%mapColumns, cell/mapColumns
		x := (cx-cy)*8 - 57
		y := (cx+cy-(mapColumns+mapRows)/2)*4 - 32
		cellOffset := grassMapPlane*mapStride + cell*2
		value := binary.LittleEndian.Uint16(mapData[cellOffset : cellOffset+2])
		ids := []byte{byte(value), byte(value >> 8)}
		for _, id := range ids {
			if id == 0 || id == 0xff || int(id) >= len(tiles) || tiles[id] == nil {
				continue
			}
			img := tiles[id]
			w, h := img.Bounds().Dx(), img.Bounds().Dy()
			placements = append(placements, tilePlacement{img, x, y})
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x+w > maxX {
				maxX = x + w
			}
			if y+h > maxY {
				maxY = y + h
			}
		}
	}
	shiftX := ScreenWidth/2 - (minX+maxX)/2
	shiftY := ScreenHeight/2 - (minY+maxY)/2
	for _, p := range placements {
		drawAt(scene, p.image, p.x+shiftX, p.y+shiftY)
	}
	// Enlarge the human sprite and place the game's standalone sword beside its
	// outstretched hand as a provisional fighter silhouette.
	fighterOp := &ebiten.DrawImageOptions{}
	fighterOp.GeoM.Scale(2, 2)
	fighterOp.GeoM.Translate(float64(ScreenWidth/2-fighter.Bounds().Dx()), float64(ScreenHeight/2-fighter.Bounds().Dy()))
	scene.DrawImage(fighter, fighterOp)
	swordOp := &ebiten.DrawImageOptions{}
	swordOp.GeoM.Scale(1.5, 1.5)
	swordOp.GeoM.Translate(370, 300)
	scene.DrawImage(sword, swordOp)
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
	return resource.DecodeIndexedRLEFrame(src, palette)
}

func decodeObjectFrame(objects, graph []byte, objectIndex int) (*ebiten.Image, error) {
	const table = 8 // Type 1 header is followed by the 512-entry list at offset 8.
	if objectIndex < 1 || objectIndex > 512 || len(objects) < table+512*8 {
		return nil, fmt.Errorf("OBJECTS.RES index table truncated")
	}
	i := (objectIndex - 1) * 8
	blockOffset := binary.LittleEndian.Uint32(objects[table+i:])
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
	objectPalettes := [...]int{56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 152, 159, 108, 107, 108, 106, 221, 105, 222, 64}
	if objectIndex <= len(objectPalettes) {
		paletteIndex = objectPalettes[objectIndex-1]
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
