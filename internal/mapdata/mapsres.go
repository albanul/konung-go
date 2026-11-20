package mapdata

import "fmt"

// MAPS.RES is the LAN version's map archive (LAN_KONG.EXE loader 0x00417DB0):
// map n (1-based, [0x009EBA30]) starts at (n-1)*MapsResEntrySize and holds a
// whole KONUNG.N (FileSize bytes) followed by its LIGHTS.N (LightsFileSize
// bytes). The installed file has 20 maps.
const MapsResEntrySize = FileSize + LightsFileSize // 0x2F120

// MapsResEntry is one map of MAPS.RES: the KONUNG.N and LIGHTS.N bytes.
type MapsResEntry struct {
	Map, Lights []byte
}

// SplitMapsRes splits MAPS.RES into its maps; entry i is map i+1.
func SplitMapsRes(data []byte) ([]MapsResEntry, error) {
	if len(data) == 0 || len(data)%MapsResEntrySize != 0 {
		return nil, fmt.Errorf("MAPS.RES has size %d, not a multiple of %d", len(data), MapsResEntrySize)
	}
	out := make([]MapsResEntry, len(data)/MapsResEntrySize)
	for i := range out {
		e := data[i*MapsResEntrySize : (i+1)*MapsResEntrySize]
		out[i] = MapsResEntry{Map: e[:FileSize], Lights: e[FileSize:]}
	}
	return out, nil
}
