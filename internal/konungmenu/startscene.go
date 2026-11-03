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
	terrainTileCount = 304
	grassFillTile    = 15
	houseObjectID    = 88
	shadowObjectID   = 89
	warriorModelID   = 0
	warriorFrames    = 6
)

type startScene struct {
	background *ebiten.Image
	warrior    []*ebiten.Image
	tick       int
}

func loadStartScene(graphPath, objectsPath string) (*startScene, error) {
	graphData, err := os.ReadFile(graphPath)
	if err != nil {
		return nil, err
	}
	tiles, err := decodeGraphTiles(graphData)
	if err != nil {
		return nil, err
	}
	objectsData, err := os.ReadFile(objectsPath)
	if err != nil {
		return nil, err
	}
	house, err := decodeStaticObjectFrame(objectsData, graphData, houseObjectID)
	if err != nil {
		return nil, err
	}
	shadow, err := decodeStaticObjectFrame(objectsData, graphData, shadowObjectID)
	if err != nil {
		return nil, err
	}
	warrior, err := decodeType0Frames(graphData, warriorModelID, warriorFrames)
	if err != nil {
		return nil, err
	}

	background := ebiten.NewImage(ScreenWidth, ScreenHeight)
	grass := tiles[grassFillTile]
	for y := -64; y < ScreenHeight+64; y += 32 {
		for x := -114; x < ScreenWidth+114; x += 56 {
			drawAt(background, grass, x, y)
		}
	}
	// The selected map's raw 88/89 references have no verified coordinates yet.
	// Use the actual house and matching shadow art in a deliberate preview setup.
	drawScaledAt(background, shadow, 140, 28, 1, 1)
	drawScaledAt(background, house, 140, 28, 1, 1)
	return &startScene{background: background, warrior: warrior}, nil
}

func (s *startScene) Update() { s.tick++ }

func (s *startScene) Draw(screen *ebiten.Image) {
	screen.DrawImage(s.background, nil)
	if len(s.warrior) == 0 {
		return
	}
	frame := s.warrior[(s.tick/8)%len(s.warrior)]
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(1.65, 1.65)
	op.GeoM.Translate(492, 344)
	screen.DrawImage(frame, op)
}

func drawScaledAt(dst, src *ebiten.Image, x, y int, scaleX, scaleY float64) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scaleX, scaleY)
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(src, op)
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

func decodeStaticObjectFrame(objects, graph []byte, objectIndex int) (*ebiten.Image, error) {
	const tableOffset, dataOffset, headerBytes = 8, 0x1008, 0x100
	if objectIndex < 1 || objectIndex > 512 || len(objects) < tableOffset+512*8 {
		return nil, fmt.Errorf("OBJECTS.RES index table truncated")
	}
	i := tableOffset + (objectIndex-1)*8
	blockOffset := binary.LittleEndian.Uint32(objects[i:])
	blockSize := binary.LittleEndian.Uint32(objects[i+4:])
	base := dataOffset + int(blockOffset)
	if base < dataOffset || blockSize < headerBytes || uint64(base)+uint64(blockSize) > uint64(len(objects)) {
		return nil, fmt.Errorf("OBJECTS.RES object %d has invalid range", objectIndex)
	}
	if binary.BigEndian.Uint32(objects[base:]) == 2 {
		return nil, fmt.Errorf("OBJECTS.RES object %d is animated, want a static image", objectIndex)
	}
	paletteOffset := binary.LittleEndian.Uint32(objects[base+20:])
	paletteIndex := paletteOffset / 512
	if paletteIndex >= 256 || len(graph) < int(paletteIndex+1)*512 {
		return nil, fmt.Errorf("OBJECTS.RES object %d has invalid palette offset %#x", objectIndex, paletteOffset)
	}
	palette, err := resource.RGB555Palette(graph[int(paletteIndex)*512 : int(paletteIndex+1)*512])
	if err != nil {
		return nil, err
	}
	img, err := decodeIndexedTile(objects[base+headerBytes:base+int(blockSize)], &palette)
	if err != nil {
		return nil, fmt.Errorf("decode OBJECTS.RES object %d: %w", objectIndex, err)
	}
	return ebiten.NewImageFromImage(img), nil
}

func decodeType0Frames(graph []byte, modelID, count int) ([]*ebiten.Image, error) {
	const (
		blockStart          = 0x1ee8b6
		metadataBytes       = 2960*2 + 1536*4 + 3840*2
		metadataOffsetTable = 2960 * 2
		paletteID           = 2 // Preview palette only; runtime assignment has not been traced.
	)
	if modelID < 0 || count < 1 || blockStart+8 > len(graph) {
		return nil, fmt.Errorf("invalid Type 0 model request")
	}
	off := blockStart
	for id := 0; id < modelID; id++ {
		if off+8 > len(graph) || binary.BigEndian.Uint32(graph[off:]) != 0 {
			return nil, fmt.Errorf("GRAPH.RES Type 0 block %d has invalid header", id)
		}
		size := int(binary.LittleEndian.Uint32(graph[off+4:]))
		if size < 8 || off+size > len(graph) {
			return nil, fmt.Errorf("GRAPH.RES Type 0 block %d has invalid size", id)
		}
		off += size
	}
	if off+8 > len(graph) || binary.BigEndian.Uint32(graph[off:]) != 0 {
		return nil, fmt.Errorf("GRAPH.RES Type 0 model %d not found", modelID)
	}
	blockSize := int(binary.LittleEndian.Uint32(graph[off+4:]))
	if blockSize < 8+metadataBytes || off+blockSize > len(graph) {
		return nil, fmt.Errorf("GRAPH.RES Type 0 model %d has invalid size", modelID)
	}
	if len(graph) < (paletteID+1)*512 {
		return nil, fmt.Errorf("GRAPH.RES palettes are truncated")
	}
	palette, err := resource.RGB555Palette(graph[paletteID*512 : (paletteID+1)*512])
	if err != nil {
		return nil, err
	}
	dataStart := off + 8 + metadataBytes
	dataBytes := blockSize - 8 - metadataBytes
	offsetsAt := off + 8 + metadataOffsetTable
	offsets := make([]uint32, count*2+2)
	for i := range offsets {
		at := offsetsAt + i*4
		if at+4 > off+8+metadataBytes {
			return nil, fmt.Errorf("GRAPH.RES Type 0 offset table truncated")
		}
		offsets[i] = binary.LittleEndian.Uint32(graph[at:])
	}
	for i := 0; i+1 < len(offsets); i += 2 {
		offsets[i], offsets[i+1] = offsets[i+1], offsets[i]
	}
	frames := make([]*ebiten.Image, 0, count)
	for i := 0; i < count; i++ {
		start := offsets[i*2]
		end := uint32(dataBytes)
		if i+1 < count {
			end = offsets[(i+1)*2]
		}
		if end <= start || uint64(end) > uint64(dataBytes) {
			return nil, fmt.Errorf("GRAPH.RES Type 0 model %d frame %d has invalid range", modelID, i)
		}
		frameData := graph[dataStart+int(start) : dataStart+int(end)]
		img, decodeErr := resource.DecodeIndexedRLEFrame(frameData, &palette)
		if decodeErr != nil {
			img, decodeErr = resource.DecodeIndexedRLELegacyFrame(frameData, &palette)
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("decode GRAPH.RES Type 0 model %d frame %d: %w", modelID, i, decodeErr)
		}
		frames = append(frames, ebiten.NewImageFromImage(img))
	}
	return frames, nil
}
