// Command sprite-catalog extracts every image entry described by the checked-in
// sprite index and creates a searchable local HTML gallery plus frame TSV.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"konung1.5/internal/gamestate"
	"konung1.5/internal/resfmt"
	"konung1.5/internal/sitenav"
)

type sourceRow struct {
	namespace, resourceName, id, kind, offset, size, width, height, palette, frames, note string
}

type catalogItem struct {
	Namespace    string `json:"namespace"`
	Resource     string `json:"resource"`
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Frame        int    `json:"frame"`
	Image        string `json:"image"`
	EntryOffset  string `json:"entry_offset"`
	FrameOffset  string `json:"frame_offset"`
	IndexedBytes uint64 `json:"indexed_bytes"`
	PayloadBytes uint64 `json:"payload_bytes"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Palette      string `json:"palette,omitempty"`
	Note         string `json:"note,omitempty"`
	Label        string `json:"label,omitempty"`
	Confidence   string `json:"confidence,omitempty"`
	Reviewer     string `json:"reviewer,omitempty"`
	LabelNotes   string `json:"label_notes,omitempty"`
	Relation     string `json:"relation,omitempty"`
}

type decodedFrame struct {
	image         *image.RGBA
	payloadOffset uint64
	payloadBytes  uint64
}

type humanLabel struct {
	label, confidence, reviewer, notes string
}

type spriteRelation struct {
	frameStart, frameEnd                int
	relation, targetNamespace, targetID string
	targetFrameStart                    int
}

var objectPalettes = [...]int{56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 152, 159, 108, 107, 108, 106, 221, 105, 222, 64}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sprite-catalog:", err)
		os.Exit(1)
	}
}

func run() error {
	root := "assets/Konung"
	out := filepath.Join(sitenav.DefaultRoot, "sprites")
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if len(os.Args) > 2 {
		out = os.Args[2]
	}
	indexFile, err := os.Open("docs/data/konung_sprite_index.tsv")
	if err != nil {
		return err
	}
	defer indexFile.Close()
	r := csv.NewReader(indexFile)
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	if _, err := r.Read(); err != nil {
		return fmt.Errorf("read index header: %w", err)
	}
	rows := make([]sourceRow, 0, 1500)
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read sprite index: %w", err)
		}
		rows = append(rows, sourceRow{record[0], record[1], record[2], record[3], record[4], record[5], record[6], record[7], record[8], record[9], record[12]})
	}
	labels, err := readLabels("docs/data/konung_sprite_labels.tsv")
	if err != nil {
		return err
	}
	relations, err := readRelations("docs/data/konung_sprite_relations.tsv")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	graph, err := os.ReadFile(filepath.Join(root, "GRAPH.RES"))
	if err != nil {
		return err
	}
	objects, err := os.ReadFile(filepath.Join(root, "OBJECTS.RES"))
	if err != nil {
		return err
	}
	paletteData, err := resfmt.Palettes(graph)
	if err != nil {
		return err
	}
	var items []catalogItem
	var failures []string
	// The exe holds the overlay palettes; without it the Type 0 frames fall
	// back to a preview palette.
	var tables *resfmt.ExeTables
	if exe, err := os.ReadFile(filepath.Join(root, "konung.exe")); err == nil {
		if tables, err = resfmt.NewExeTables(exe); err != nil {
			return err
		}
	}
	type0Items, type0Failures, err := extractGraphType0(graph, out, paletteData, tables)
	if err != nil {
		return err
	}
	items = append(items, type0Items...)
	failures = append(failures, type0Failures...)
	for _, row := range rows {
		if row.kind == "unused table slot" || row.kind == "unused" {
			continue
		}
		var decoded []decodedFrame
		switch row.namespace {
		case "GRAPH.T5", "GRAPH.T1", "TYPE5":
			decoded, err = decodeImageRow(row, root, graph, paletteData)
		case "OBJECTS":
			if row.kind == "animated object" {
				decoded, err = decodeObject(row, objects, paletteData)
			} else {
				var frame decodedFrame
				frame, err = decodeStaticObject(row, objects, paletteData)
				if frame.image != nil {
					decoded = []decodedFrame{frame}
				}
			}
		default:
			continue // map-reference and other metadata rows are not images
		}
		if err != nil {
			failures = append(failures, strings.Join([]string{row.namespace, row.id, err.Error()}, "\t"))
			continue
		}
		entryOffset, _ := number(row.offset)
		indexedBytes, _ := number(row.size)
		for frame, decodedImage := range decoded {
			img := decodedImage.image
			rel := imagePath(row, frame, len(decoded))
			full := filepath.Join(out, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
				return err
			}
			f, err := os.Create(full)
			if err != nil {
				return err
			}
			err = png.Encode(f, img)
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			label := labels[labelKey(row.namespace, row.id, strconv.Itoa(frame))]
			if label.label == "" {
				label = labels[labelKey(row.namespace, row.id, "*")]
			}
			relation := relationFor(relations, row.namespace, row.id, frame)
			items = append(items, catalogItem{
				Namespace: row.namespace, Resource: row.resourceName, ID: row.id, Kind: row.kind, Frame: frame, Image: rel,
				EntryOffset: hexOffset(entryOffset), FrameOffset: hexOffset(decodedImage.payloadOffset), IndexedBytes: indexedBytes, PayloadBytes: decodedImage.payloadBytes,
				Width: img.Bounds().Dx(), Height: img.Bounds().Dy(), Palette: row.palette, Note: row.note,
				Label: label.label, Confidence: label.confidence, Reviewer: label.reviewer, LabelNotes: label.notes, Relation: relation,
			})
		}
		if len(items)%500 == 0 {
			fmt.Printf("extracted %d images\n", len(items))
		}
	}
	if err := writeCatalog(out, items); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "errors.tsv"), []byte("namespace\tsprite_id\terror\n"+strings.Join(failures, "\n")), 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %d image frames to %s (%d extraction errors)\n", len(items), out, len(failures))
	return nil
}

func readRelations(path string) (map[string][]spriteRelation, error) {
	relations := make(map[string][]spriteRelation)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	if _, err := r.Read(); err != nil {
		return nil, err
	}
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(row) < 10 {
			return nil, fmt.Errorf("malformed relation row")
		}
		start, err := strconv.Atoi(row[2])
		if err != nil {
			return nil, err
		}
		end, err := strconv.Atoi(row[3])
		if err != nil {
			return nil, err
		}
		targetStart, err := strconv.Atoi(row[7])
		if err != nil {
			return nil, err
		}
		relations[labelKey(row[0], row[1], "*")] = append(relations[labelKey(row[0], row[1], "*")], spriteRelation{start, end, row[4], row[5], row[6], targetStart})
	}
	return relations, nil
}

func relationFor(relations map[string][]spriteRelation, namespace, id string, frame int) string {
	for _, relation := range relations[labelKey(namespace, id, "*")] {
		if frame < relation.frameStart || frame > relation.frameEnd {
			continue
		}
		targetFrame := relation.targetFrameStart + frame - relation.frameStart
		return fmt.Sprintf("Exact duplicate of %s %s frame %d", relation.targetNamespace, relation.targetID, targetFrame)
	}
	return ""
}

func readLabels(path string) (map[string]humanLabel, error) {
	labels := make(map[string]humanLabel)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return labels, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	if _, err := r.Read(); err != nil {
		return nil, err
	}
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) < 7 || record[3] == "" {
			continue
		}
		labels[labelKey(record[0], record[1], record[2])] = humanLabel{record[3], record[4], record[5], record[6]}
	}
	return labels, nil
}

func labelKey(namespace, id, frame string) string {
	return namespace + "\x00" + id + "\x00" + frame
}

// type0Palette is the palette the game draws Type 0 block id with, and a
// label for it. Bodies (blocks 0–2) take the entity's +0xD8; the catalog
// shows the hero of that body (gamestate.HeroPalette), and NPCs of the same
// body use other palettes of its family (docs/konung_graph_sprites.md).
// Overlays (blocks 3–26) have a fixed palette from the exe table
// 0x00465DCC. Without the exe, overlays get preview palette 2.
func type0Palette(id int, t *resfmt.ExeTables) (int, string) {
	switch {
	case id < len(gamestate.HeroPalette):
		p := int(gamestate.HeroPalette[id] / 512)
		return p, fmt.Sprintf("%d (body %d: hero palette; NPCs use their own +0xD8)", p, id)
	case t != nil:
		p := int(t.OverlayPalette(id-len(gamestate.HeroPalette)) / 512)
		return p, fmt.Sprintf("%d (overlay %d: exe table 0x00465DCC)", p, id-len(gamestate.HeroPalette))
	default:
		return 2, "2 (preview: konung.exe not found)"
	}
}

// extractGraphType0 preserves the validated Type 0 blocks and decodes their
// indexed-RLE frames with the palette the game uses (type0Palette). The
// block size includes its 8-byte header.
func extractGraphType0(graph []byte, out string, palettes [][256]color.RGBA, tables *resfmt.ExeTables) ([]catalogItem, []string, error) {
	const type0MetadataBytes = resfmt.Type0DataOffset - 8 // payload metadata, excluding the 8-byte header
	blocks, err := resfmt.Type0Blocks(graph)
	if err != nil {
		return nil, nil, err
	}
	dir := filepath.Join(out, "raw", "GRAPH.T0")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, nil, err
	}
	var items []catalogItem
	var failures []string
	var inventory bytes.Buffer
	w := csv.NewWriter(&inventory)
	w.Comma = '\t'
	_ = w.Write([]string{"block_id", "file", "block_offset", "block_bytes", "payload_bytes", "type0_metadata_bytes", "remaining_payload_bytes", "offset_table_nonzero", "block_end", "decoded_frames", "decode_failures", "palette"})
	for id, blk := range blocks {
		off, size, end := blk.Offset, uint64(len(blk.Data)), blk.Offset+len(blk.Data)
		payloadBytes := int(size) - 8
		// CTPAX-X's source describes a 1,536-entry uint32 offset table at
		// byte 5,920 in the payload. Count nonzero slots as a sanity signal,
		// while keeping the format hypothesis explicit in the inventory.
		nonzeroOffsets := 0
		for i := 0; i < 1536; i++ {
			if binary.LittleEndian.Uint32(graph[off+8+5920+i*4:]) != 0 {
				nonzeroOffsets++
			}
		}
		name := fmt.Sprintf("block_%03d.bin", id)
		if err := os.WriteFile(filepath.Join(dir, name), graph[off:end], 0644); err != nil {
			return nil, nil, err
		}
		dataStart := off + resfmt.Type0DataOffset
		dataBytes := payloadBytes - type0MetadataBytes
		decodedFrames, decodeFailures := 0, 0
		pal, palLabel := type0Palette(id, tables)
		for frameIndex, fr := range resfmt.FrameRanges(blk.Data[resfmt.Type0PairsOffset:resfmt.Type0DataOffset], dataBytes) {
			frameStart, frameEnd := fr.Start, fr.End
			if frameEnd <= frameStart || frameEnd > uint32(dataBytes) {
				failures = append(failures, fmt.Sprintf("GRAPH.T0\t%d\tframe %d has invalid range %d..%d", id, frameIndex, frameStart, frameEnd))
				decodeFailures++
				continue
			}
			frameData := graph[dataStart+int(frameStart) : dataStart+int(frameEnd)]
			img, err := resfmt.DecodeIndexedRLEFrame(frameData, &palettes[pal])
			if err != nil {
				img, err = resfmt.DecodeIndexedRLELegacyFrame(frameData, &palettes[pal])
			}
			if err != nil {
				failures = append(failures, fmt.Sprintf("GRAPH.T0\t%d\tframe %d: %v", id, frameIndex, err))
				decodeFailures++
				continue
			}
			imageName := fmt.Sprintf("images/GRAPH.T0/%03d/frame_%03d.png", id, frameIndex)
			fullImagePath := filepath.Join(out, filepath.FromSlash(imageName))
			if err := os.MkdirAll(filepath.Dir(fullImagePath), 0755); err != nil {
				return nil, nil, err
			}
			f, err := os.Create(fullImagePath)
			if err != nil {
				return nil, nil, err
			}
			err = png.Encode(f, img)
			closeErr := f.Close()
			if err != nil {
				return nil, nil, err
			}
			if closeErr != nil {
				return nil, nil, closeErr
			}
			items = append(items, catalogItem{
				Namespace: "GRAPH.T0", Resource: "GRAPH.RES", ID: strconv.Itoa(id), Kind: "Type 0 animation", Frame: frameIndex,
				Image: imageName, EntryOffset: hexOffset(uint64(off)), FrameOffset: hexOffset(uint64(dataStart + int(frameStart))),
				IndexedBytes: size, PayloadBytes: uint64(frameEnd - frameStart), Width: img.Bounds().Dx(), Height: img.Bounds().Dy(),
				Palette: palLabel, Note: "Type 0 indexed-RLE frame",
			})
			decodedFrames++
		}
		_ = w.Write([]string{
			strconv.Itoa(id), filepath.ToSlash(filepath.Join("raw", "GRAPH.T0", name)), hexOffset(uint64(off)),
			strconv.FormatUint(size, 10), strconv.Itoa(payloadBytes), strconv.Itoa(type0MetadataBytes),
			strconv.Itoa(dataBytes), strconv.Itoa(nonzeroOffsets), hexOffset(uint64(end)), strconv.Itoa(decodedFrames), strconv.Itoa(decodeFailures), palLabel,
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(filepath.Join(out, "type0.tsv"), inventory.Bytes(), 0644); err != nil {
		return nil, nil, err
	}
	return items, failures, nil
}

func decodeImageRow(row sourceRow, root string, graph []byte, palettes [][256]color.RGBA) ([]decodedFrame, error) {
	off, err := number(row.offset)
	if err != nil {
		return nil, err
	}
	sz, err := number(row.size)
	if err != nil {
		return nil, err
	}
	var data []byte
	if row.namespace == "GRAPH.T1" {
		if off+sz > uint64(len(graph)) {
			return nil, fmt.Errorf("GRAPH.T1 range exceeds GRAPH.RES")
		}
		pal, err := number(row.palette)
		if err != nil || pal >= uint64(len(palettes)) {
			return nil, fmt.Errorf("invalid palette index %q", row.palette)
		}
		img, err := resfmt.DecodeIndexedRLEFrame(graph[off:off+sz], &palettes[pal])
		if err != nil {
			img, err = resfmt.DecodeIndexedRLELegacyFrame(graph[off:off+sz], &palettes[pal])
			if err != nil {
				return nil, err
			}
		}
		return []decodedFrame{{image: img, payloadOffset: off, payloadBytes: sz}}, nil
	}
	var payloadOffset uint64
	if row.namespace == "GRAPH.T5" {
		if off+8 > uint64(len(graph)) || binary.BigEndian.Uint32(graph[off:off+4]) != 5 {
			return nil, fmt.Errorf("invalid GRAPH Type 5 block header")
		}
		frameSize := binary.LittleEndian.Uint32(graph[off+4 : off+8])
		if uint64(frameSize)+8+off > uint64(len(graph)) {
			return nil, fmt.Errorf("GRAPH Type 5 frame exceeds file")
		}
		data = graph[off+8 : off+8+uint64(frameSize)]
		payloadOffset = off + 8
	} else {
		file, err := os.ReadFile(filepath.Join(root, row.resourceName))
		if err != nil {
			return nil, err
		}
		if off+8 > uint64(len(file)) || binary.BigEndian.Uint32(file[off:off+4]) != 5 {
			return nil, fmt.Errorf("invalid Type 5 frame header")
		}
		frameSize := binary.LittleEndian.Uint32(file[off+4 : off+8])
		if uint64(frameSize)+8+off > uint64(len(file)) {
			return nil, fmt.Errorf("Type 5 frame exceeds file")
		}
		data = file[off+8 : off+8+uint64(frameSize)]
		payloadOffset = off + 8
	}
	img, err := resfmt.DecodeRLEFrame(data)
	if err != nil {
		return nil, err
	}
	return []decodedFrame{{image: img, payloadOffset: payloadOffset, payloadBytes: uint64(len(data))}}, nil
}

func decodeObject(row sourceRow, objects []byte, palettes [][256]color.RGBA) ([]decodedFrame, error) {
	base64, err := number(row.offset)
	if err != nil {
		return nil, err
	}
	size64, err := number(row.size)
	if err != nil {
		return nil, err
	}
	base, size := int(base64), int(size64)
	const headerSize = 0x3228
	if base < 0 || size < headerSize || base+size > len(objects) {
		return nil, fmt.Errorf("object block range invalid")
	}
	if binary.BigEndian.Uint32(objects[base:]) != 2 {
		return nil, fmt.Errorf("animated object block is not Type 2")
	}
	palIndex, err := objectPalette(row.id)
	if err != nil || palIndex >= len(palettes) {
		return nil, fmt.Errorf("invalid animation palette %d", palIndex)
	}
	// Type 2 stores paired (shadow, image) offsets; see resfmt.FrameRanges.
	dataStart := uint32(resfmt.Type2DataOffset)
	dataSize := uint32(size - headerSize)
	var result []decodedFrame
	for _, fr := range resfmt.FrameRanges(objects[base+resfmt.Type2PairsOffset:base+resfmt.Type2PairsOffset+512*8], int(dataSize)) {
		start, end := fr.Start, fr.End
		if end <= start || end > dataSize {
			return nil, fmt.Errorf("frame %d has invalid range %d..%d", len(result), start, end)
		}
		frame := objects[base+int(dataStart+start) : base+int(dataStart+end)]
		img, err := resfmt.DecodeIndexedRLEFrame(frame, &palettes[palIndex])
		if err != nil {
			return nil, fmt.Errorf("frame %d: %w", len(result), err)
		}
		result = append(result, decodedFrame{image: img, payloadOffset: base64 + uint64(dataStart) + uint64(start), payloadBytes: uint64(end - start)})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("object has no image frames")
	}
	return result, nil
}

func decodeStaticObject(row sourceRow, objects []byte, palettes [][256]color.RGBA) (decodedFrame, error) {
	base64, err := number(row.offset)
	if err != nil {
		return decodedFrame{}, err
	}
	size64, err := number(row.size)
	if err != nil {
		return decodedFrame{}, err
	}
	base, size := int(base64), int(size64)
	const payloadOffset = 0x100
	if base < 0 || size < payloadOffset || base+size > len(objects) {
		return decodedFrame{}, fmt.Errorf("object block range invalid")
	}
	paletteOffset := binary.LittleEndian.Uint32(objects[base+20 : base+24])
	if paletteOffset == ^uint32(0) {
		return decodedFrame{}, fmt.Errorf("object palette offset is unset")
	}
	paletteIndex := paletteOffset / 512
	if paletteIndex >= uint32(len(palettes)) {
		return decodedFrame{}, fmt.Errorf("object palette offset %d is out of range", paletteOffset)
	}
	data := objects[base+payloadOffset : base+size]
	img, err := resfmt.DecodeIndexedRLEFrame(data, &palettes[paletteIndex])
	if err != nil {
		return decodedFrame{}, err
	}
	return decodedFrame{image: img, payloadOffset: base64 + uint64(payloadOffset), payloadBytes: uint64(len(data))}, nil
}

func objectPalette(id string) (int, error) {
	n, err := strconv.Atoi(id)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid object id %q", id)
	}
	if n <= len(objectPalettes) {
		return objectPalettes[n-1], nil
	}
	return objectPalettes[0], nil
}

func number(s string) (uint64, error) {
	if s == "" {
		return 0, fmt.Errorf("missing numeric field")
	}
	n, err := strconv.ParseUint(strings.TrimSpace(s), 0, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func imagePath(row sourceRow, frame, _ int) string {
	id, _ := strconv.Atoi(row.id)
	switch row.namespace {
	case "OBJECTS":
		return fmt.Sprintf("images/OBJECTS/%04d/frame_%03d.png", id, frame)
	case "GRAPH.T1":
		return fmt.Sprintf("images/GRAPH.T1/%03d.png", id)
	case "GRAPH.T5":
		return fmt.Sprintf("images/GRAPH.T5/%03d.png", id)
	default:
		return fmt.Sprintf("images/%s/%03d.png", row.resourceName, id)
	}
}

func writeCatalog(out string, items []catalogItem) error {
	var tsv bytes.Buffer
	w := csv.NewWriter(&tsv)
	w.Comma = '\t'
	_ = w.Write([]string{"namespace", "resource", "sprite_id", "kind", "frame", "image", "entry_offset", "frame_offset", "indexed_bytes", "payload_bytes", "width", "height", "palette_id", "visual_note", "human_label", "confidence", "reviewer", "label_notes", "relation"})
	for _, item := range items {
		_ = w.Write([]string{item.Namespace, item.Resource, item.ID, item.Kind, strconv.Itoa(item.Frame), item.Image, item.EntryOffset, item.FrameOffset, strconv.FormatUint(item.IndexedBytes, 10), strconv.FormatUint(item.PayloadBytes, 10), strconv.Itoa(item.Width), strconv.Itoa(item.Height), item.Palette, item.Note, item.Label, item.Confidence, item.Reviewer, item.LabelNotes, item.Relation})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "catalog.tsv"), tsv.Bytes(), 0644); err != nil {
		return err
	}
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	page := `<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>Konung sprite catalog</title>
<style>
__NAVSTYLE__
body{font:14px system-ui;margin:20px;background:#1d2024;color:#eee}input,select{font:inherit;padding:8px;margin:4px}
#grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(170px,1fr));gap:10px}
.card{background:#30353b;padding:8px;border-radius:5px;min-width:0}.card img{width:100%;height:130px;object-fit:contain;image-rendering:pixelated;background:repeating-conic-gradient(#454a50 0% 25%,#383d43 0% 50%) 50%/16px 16px}
.meta{overflow-wrap:anywhere}.muted{color:#bbc2ca}.notice{padding:12px;background:#513b22;border-left:4px solid #e4a84d;max-width:1100px}.frames{margin-top:8px}.framegrid{display:grid;grid-template-columns:repeat(auto-fill,minmax(105px,1fr));gap:8px;margin-top:8px}.framecard{background:#25292e;padding:5px;border-radius:4px;overflow-wrap:anywhere}.framecard img{height:90px}small{color:#c4c9cf}button{padding:8px;margin:8px}
</style></head><body>
__NAV__
<h1>Konung image catalog</h1>
<p class="notice"><b>Note.</b> GRAPH Type 0 animation frames are decoded and grouped below with the game's palettes: bodies with their hero's palette (NPCs of the same body use other palettes of its family, entity +0xD8), overlays with the exe table 0x00465DCC. Complete raw blocks and offsets are in <a href="type0.tsv">type0.tsv</a>.</p>
<p>Animated resources are grouped by model. Open “Explore frames” on a card to inspect the animation and directions.</p>
<input id="q" placeholder="Search ID, note, label, or offset">
<select id="ns"><option value="">All image entries</option></select><p id="count"></p>
<div id="grid"></div><button id="more">Load more</button>
<script>
const data=__DATA__;
const ns=[...new Set(data.map(x=>x.namespace))];
const sel=document.querySelector('#ns');
for(const n of ns){const o=document.createElement('option');o.value=n;o.textContent=(n==='OBJECTS'||n==='GRAPH.T0')?n+' (grouped models)':n;sel.append(o)}
sel.value='OBJECTS';
let limit=300;
function esc(v){return String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
function searchable(x){return [x.id,x.note,x.label,x.label_notes,x.relation,x.resource,x.namespace,x.entry_offset,x.frame_offset].join(' ').toLowerCase()}
function imageCard(x, frameMode){const title=esc(x.label||x.note||x.resource);const frameLabel=frameMode?'<br>frame '+x.frame+' · '+esc(x.frame_offset):'';const relation=x.relation?'<br><span class="muted">'+esc(x.relation)+'</span>':'';return '<div class="'+(frameMode?'framecard':'card')+'"><a href="'+esc(x.image)+'"><img loading="lazy" src="'+esc(x.image)+'"></a><div class="meta"><b>'+esc(x.namespace)+' '+esc(x.id)+(frameMode?' / frame '+x.frame:'')+'</b><br>'+x.width+'×'+x.height+'<br><span class="muted">'+title+'</span>'+frameLabel+relation+(frameMode?'':'<br><small>'+esc(x.resource)+'<br>entry '+esc(x.entry_offset)+' · frame '+esc(x.frame_offset)+'</small>')+'</div></div>'}
function renderGrouped(namespace,q){const groups=new Map();for(const x of data){if(x.namespace!==namespace)continue;if(!groups.has(x.id))groups.set(x.id,[]);groups.get(x.id).push(x)}const matches=[...groups.values()].filter(g=>!q||g.some(x=>searchable(x).includes(q)));document.querySelector('#count').textContent=matches.length+' models ('+matches.reduce((n,g)=>n+g.length,0)+' animation frames)';document.querySelector('#grid').innerHTML=matches.map(g=>{const first=g.find(x=>x.frame===0)||g[0];const allFrames=g.map(x=>imageCard(x,true)).join('');return '<div class="card">'+imageCard(first,false)+'<details class="frames"><summary>Explore '+g.length+' frames</summary><div class="framegrid">'+allFrames+'</div></details></div>'}).join('');document.querySelector('#more').hidden=true}
function render(){const q=document.querySelector('#q').value.toLowerCase(),n=sel.value;if(n==='OBJECTS'||n==='GRAPH.T0'){renderGrouped(n,q);return}const rows=data.filter(x=>(!n||x.namespace===n)&&(!q||searchable(x).includes(q)));document.querySelector('#count').textContent=rows.length+' images ('+data.length+' extracted total)';document.querySelector('#grid').innerHTML=rows.slice(0,limit).map(x=>imageCard(x,false)).join('');document.querySelector('#more').hidden=rows.length<=limit}
document.querySelector('#q').oninput=()=>{limit=300;render()};sel.onchange=()=>{limit=300;render()};document.querySelector('#more').onclick=()=>{limit+=300;render()};render();
</script></body></html>`
	root, inSite := sitenav.SectionRoot(out)
	navStyle, nav := "", ""
	if inSite {
		navStyle, nav = sitenav.Style, string(sitenav.Nav("sprites"))
	}
	page = strings.Replace(page, "__NAVSTYLE__", navStyle, 1)
	page = strings.Replace(page, "__NAV__", nav, 1)
	page = strings.Replace(page, "__DATA__", string(data), 1)
	pagePath := filepath.Join(out, "index.html")
	if err := os.WriteFile(pagePath, []byte(page), 0644); err != nil {
		return err
	}
	if inSite {
		return sitenav.WriteHub(root)
	}
	return nil
}

func hexOffset(offset uint64) string {
	return fmt.Sprintf("0x%08X", offset)
}
