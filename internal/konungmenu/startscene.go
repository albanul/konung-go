package konungmenu

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"konung1.5/internal/konungmap"
	"konung1.5/internal/resource"
)

const (
	houseObjectID  = 88
	shadowObjectID = 89
	warriorModelID = 0
	warriorFrames  = 6
	warriorScale   = 1.65
	scrollSpeed    = 12
	edgeScrollZone = 8
)

// Preview placements in world pixels. The house and warrior are not decoded
// from KONUNG.1's object/entity sections yet; they are hand-placed near the
// road so the terrain render has a focal point.
var (
	housePos   = image.Pt(2900, 2500)
	warriorPos = image.Pt(3330, 2800)
)

type startScene struct {
	m              *konungmap.Map
	tiles          []*ebiten.Image
	tileMarginX    int
	tileMarginY    int
	worldW, worldH int
	house, shadow  *ebiten.Image
	warrior        []*ebiten.Image
	camX, camY     int
	tick           int
}

func loadStartScene(graphPath, objectsPath, mapPath string) (*startScene, error) {
	graphData, err := os.ReadFile(graphPath)
	if err != nil {
		return nil, err
	}
	objectsData, err := os.ReadFile(objectsPath)
	if err != nil {
		return nil, err
	}
	mapData, err := os.ReadFile(mapPath)
	if err != nil {
		return nil, err
	}
	m, err := konungmap.Parse(mapData)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", mapPath, err)
	}
	rawTiles, err := konungmap.LoadTerrainTiles(graphData)
	if err != nil {
		return nil, err
	}
	s := &startScene{m: m, tiles: make([]*ebiten.Image, len(rawTiles))}
	// Only upload tiles the map references; slot 304 is a large non-tile image.
	for r := range m.Cells {
		for c := range m.Cells[r] {
			for _, b := range m.Cells[r][c] {
				t := konungmap.Tile(rawTiles, b)
				if t == nil || s.tiles[b-1] != nil {
					continue
				}
				s.tiles[b-1] = ebiten.NewImageFromImage(t)
				s.tileMarginX = max(s.tileMarginX, t.Bounds().Dx())
				s.tileMarginY = max(s.tileMarginY, t.Bounds().Dy())
			}
		}
	}
	s.worldW, s.worldH = m.WorldSize()
	if s.house, err = decodeStaticObjectFrame(objectsData, graphData, houseObjectID); err != nil {
		return nil, err
	}
	if s.shadow, err = decodeStaticObjectFrame(objectsData, graphData, shadowObjectID); err != nil {
		return nil, err
	}
	if s.warrior, err = decodeType0Frames(graphData, warriorModelID, warriorFrames); err != nil {
		return nil, err
	}
	s.centerOn(warriorPos.X, warriorPos.Y)
	return s, nil
}

func (s *startScene) centerOn(x, y int) {
	s.camX, s.camY = x-ScreenWidth/2, y-ScreenHeight/2
	s.clampCamera()
}

func (s *startScene) clampCamera() {
	s.camX = max(0, min(s.camX, s.worldW-ScreenWidth))
	s.camY = max(0, min(s.camY, s.worldH-ScreenHeight))
}

func (s *startScene) Update() {
	s.tick++
	mx, my := ebiten.CursorPosition()
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || ebiten.IsKeyPressed(ebiten.KeyA) || mx < edgeScrollZone {
		s.camX -= scrollSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) || ebiten.IsKeyPressed(ebiten.KeyD) || mx >= ScreenWidth-edgeScrollZone {
		s.camX += scrollSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW) || my < edgeScrollZone {
		s.camY -= scrollSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyS) || my >= ScreenHeight-edgeScrollZone {
		s.camY += scrollSpeed
	}
	s.clampCamera()
}

func (s *startScene) Draw(screen *ebiten.Image) {
	screen.Fill(color.Black)
	// Same order as the game's terrain pass (VA 0x0042562F): rows top to
	// bottom, columns left to right, base layer then overlay layer.
	rowMin := max(0, (s.camY-s.tileMarginY)/konungmap.CellHeight)
	rowMax := min(konungmap.Rows-1, (s.camY+ScreenHeight)/konungmap.CellHeight)
	colMin := max(0, (s.camX-s.tileMarginX)/konungmap.CellWidth)
	colMax := min(konungmap.Cols-1, (s.camX+ScreenWidth)/konungmap.CellWidth)
	for r := rowMin; r <= rowMax; r++ {
		for c := colMin; c <= colMax; c++ {
			x, y := konungmap.CellOrigin(r, c)
			for _, b := range s.m.Cells[r][c] {
				if b != 0 && int(b) <= len(s.tiles) && s.tiles[b-1] != nil {
					drawAt(screen, s.tiles[b-1], x-s.camX, y-s.camY)
				}
			}
		}
	}
	drawAt(screen, s.shadow, housePos.X-s.camX, housePos.Y-s.camY)
	drawAt(screen, s.house, housePos.X-s.camX, housePos.Y-s.camY)
	if len(s.warrior) > 0 {
		frame := s.warrior[(s.tick/8)%len(s.warrior)]
		drawScaledAt(screen, frame, warriorPos.X-s.camX, warriorPos.Y-s.camY, warriorScale, warriorScale)
	}
}

func drawScaledAt(dst, src *ebiten.Image, x, y int, scaleX, scaleY float64) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scaleX, scaleY)
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(src, op)
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
	img, err := resource.DecodeIndexedRLEFrame(objects[base+headerBytes:base+int(blockSize)], &palette)
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
