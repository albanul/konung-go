package resource

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
