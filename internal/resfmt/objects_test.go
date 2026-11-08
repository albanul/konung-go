package resfmt

import (
	"os"
	"testing"
)

func TestDecodeSceneAssets(t *testing.T) {
	graph, err := os.ReadFile("../../assets/Konung/GRAPH.RES")
	if err != nil {
		t.Skip("GRAPH.RES not available:", err)
	}
	objects, err := os.ReadFile("../../assets/Konung/OBJECTS.RES")
	if err != nil {
		t.Skip("OBJECTS.RES not available:", err)
	}
	house, err := DecodeStaticObject(objects, graph, 88)
	if err != nil {
		t.Fatal(err)
	}
	if b := house.Bounds(); b.Dx() != 494 || b.Dy() != 453 {
		t.Errorf("house 88 is %v, want 494x453", b)
	}
	if _, err := DecodeStaticObject(objects, graph, 1); err == nil {
		t.Error("object 1 is animated; want an error from DecodeStaticObject")
	}
	frames, err := DecodeType0Frames(graph, 0, 6, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 6 {
		t.Fatalf("got %d frames, want 6", len(frames))
	}
}

func TestLoadMapObjectWithShadow(t *testing.T) {
	graph, err := os.ReadFile("../../assets/Konung/GRAPH.RES")
	if err != nil {
		t.Skip("GRAPH.RES not available:", err)
	}
	objects, err := os.ReadFile("../../assets/Konung/OBJECTS.RES")
	if err != nil {
		t.Skip("OBJECTS.RES not available:", err)
	}
	// KONUNG.1's first placement: ID 117 -> slot 147, a log house.
	obj, err := LoadMapObject(objects, graph, MapObjectSlot(117), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if b := obj.Image.Bounds(); b.Dx() != 454 || b.Dy() != 378 {
		t.Errorf("image %v, want 454x378", b)
	}
	if obj.ImageDX != -225 || obj.ImageDY != -346 || obj.ShadowDX != -432 || obj.ShadowDY != -180 {
		t.Errorf("offsets image %d,%d shadow %d,%d", obj.ImageDX, obj.ImageDY, obj.ShadowDX, obj.ShadowDY)
	}
	if obj.Shadow == nil {
		t.Fatal("expected a shadow mask")
	}
	covered := 0
	for _, a := range obj.Shadow.Pix {
		if a != 0 {
			covered++
		}
	}
	if covered == 0 {
		t.Error("shadow mask is empty")
	}
}

// Every frame of the 27 Type 0 blocks decodes, 14067 in all (the sprite
// catalog's count), and the frame ranges lie inside the image data.
func TestDecodeType0BlockAll(t *testing.T) {
	graph, err := os.ReadFile("../../assets/Konung/GRAPH.RES")
	if err != nil {
		t.Skip("GRAPH.RES not available:", err)
	}
	blocks, err := Type0Blocks(graph)
	if err != nil {
		t.Fatal(err)
	}
	pals, err := Palettes(graph)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 27 {
		t.Fatalf("got %d Type 0 blocks, want 27", len(blocks))
	}
	total := 0
	for id, b := range blocks {
		for i, f := range DecodeType0Block(b.Data, &pals[2], -1) {
			if f.Err != nil || f.Image == nil {
				t.Fatalf("block %d frame %d: %v", id, i, f.Err)
			}
			if f.Start < Type0DataOffset || f.End > len(b.Data) {
				t.Fatalf("block %d frame %d range %d..%d outside the data", id, i, f.Start, f.End)
			}
			total++
		}
	}
	if total != 14067 {
		t.Errorf("decoded %d frames, want 14067", total)
	}
	if n := len(DecodeType0Block(blocks[0].Data, &pals[2], 6)); n != 6 {
		t.Errorf("limit 6 gave %d frames", n)
	}
}
