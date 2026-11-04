package resfmt

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const soundEntryCount = 1000

// SoundEntry is one sample from SOUNDS.RES or Sounds_.res. Index is zero-based.
type SoundEntry struct {
	Index int
	Data  []byte
}

// LoadSoundEntries parses the fixed 1000-entry sound directory used by the
// original game. The tag is big-endian 1; the following data-section size and
// each directory record (offset,size) are little-endian uint32 values.
func LoadSoundEntries(r io.ReaderAt, fileSize int64) ([]SoundEntry, error) {
	const headerSize = 8
	const directorySize = soundEntryCount * 8
	const dataStart = headerSize + directorySize
	if fileSize < dataStart {
		return nil, fmt.Errorf("sound resource is too small: %d bytes", fileSize)
	}
	header := make([]byte, headerSize)
	if _, err := r.ReadAt(header, 0); err != nil {
		return nil, fmt.Errorf("read sound header: %w", err)
	}
	if version, size := binary.BigEndian.Uint32(header), binary.LittleEndian.Uint32(header[4:]); version != 1 {
		return nil, fmt.Errorf("unsupported sound resource version %d", version)
	} else if int64(size)+dataStart != fileSize {
		return nil, fmt.Errorf("sound resource size mismatch: header says %d data bytes, file has %d", size, fileSize-dataStart)
	}
	dir := make([]byte, directorySize)
	if _, err := r.ReadAt(dir, headerSize); err != nil {
		return nil, fmt.Errorf("read sound directory: %w", err)
	}
	entries := make([]SoundEntry, 0, soundEntryCount)
	for i := 0; i < soundEntryCount; i++ {
		record := dir[i*8 : i*8+8]
		off := binary.LittleEndian.Uint32(record)
		size := binary.LittleEndian.Uint32(record[4:])
		if off == ^uint32(0) && size == ^uint32(0) {
			continue
		}
		if off == ^uint32(0) || size == ^uint32(0) {
			return nil, fmt.Errorf("sound entry %d has a partial unused marker", i)
		}
		start, end := uint64(dataStart)+uint64(off), uint64(dataStart)+uint64(off)+uint64(size)
		if end > uint64(fileSize) || end < start {
			return nil, fmt.Errorf("sound entry %d range [%d,%d) exceeds file size %d", i, start, end, fileSize)
		}
		data := make([]byte, int(size))
		if _, err := r.ReadAt(data, int64(start)); err != nil {
			return nil, fmt.Errorf("read sound entry %d: %w", i, err)
		}
		entries = append(entries, SoundEntry{Index: i, Data: data})
	}
	return entries, nil
}

// WriteWAV writes a mono PCM WAV at the format used by Konung sound effects
// (16-bit, 22050 Hz). The sample bytes are stored verbatim.
func (e SoundEntry) WriteWAV(w io.Writer) error {
	const rate, channels, bits = uint32(22050), uint16(1), uint16(16)
	if uint64(len(e.Data)) > uint64(^uint32(0))-36 {
		return fmt.Errorf("sound entry %d is too large for RIFF/WAVE", e.Index)
	}
	var h bytes.Buffer
	h.WriteString("RIFF")
	_ = binary.Write(&h, binary.LittleEndian, uint32(len(e.Data)+36))
	h.WriteString("WAVEfmt ")
	_ = binary.Write(&h, binary.LittleEndian, uint32(16))
	_ = binary.Write(&h, binary.LittleEndian, uint16(1))
	_ = binary.Write(&h, binary.LittleEndian, channels)
	_ = binary.Write(&h, binary.LittleEndian, rate)
	_ = binary.Write(&h, binary.LittleEndian, rate*uint32(channels)*uint32(bits/8))
	_ = binary.Write(&h, binary.LittleEndian, channels*(bits/8))
	_ = binary.Write(&h, binary.LittleEndian, bits)
	h.WriteString("data")
	_ = binary.Write(&h, binary.LittleEndian, uint32(len(e.Data)))
	if _, err := w.Write(h.Bytes()); err != nil {
		return err
	}
	_, err := w.Write(e.Data)
	return err
}
