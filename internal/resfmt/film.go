package resfmt

import (
	"encoding/binary"
	"fmt"
	"image"
	"path/filepath"
	"strings"

	"github.com/gen2brain/gav1d/av1"
)

// Films (docs/konung_films.md). The game plays VIDEO\<name>.FLM through
// MP.DLL (0x0042B690), a DirectShow wrapper, so Windows decodes them. The
// installed .FLM files are AVI with WMV3 video (640×360, 12 fps) and mono
// 16-bit PCM at 22050 Hz. No pure-Go WMV3 decoder exists, so cmd/video-convert
// rewrites each film once into <name>.ivf (AV1) and <name>.ogg (Vorbis) next
// to it; FilmDecoder plays the AV1 part.

// Film names as the exe builds them (lower case, as the files are installed).
const (
	FilmPoster   = "poster"   // 0x00450689, start-up (state 9)
	FilmGameOver = "gameover" // 0x00450650, the hero's death
	FilmWin      = "win"      // 0x00450004, the index-0 pile
)

// DefeatFilm is "DEFEAT" + the current map number + ".FLM" (0x0041D781), the
// village surrender on map m.
func DefeatFilm(m int) string { return fmt.Sprintf("defeat%d", m) }

// FilmRect is the window 0x0042B690 asks MP.DLL for: ",80,120,640,360"
// (0x00450273), x, y, width, height on the 800×600 screen.
var FilmRect = image.Rect(80, 120, 80+640, 120+360)

// FilmPaths returns the converted video and audio files of a film in the
// game directory (VIDEO\ is 0x0045026C).
func FilmPaths(gameDir, name string) (video, audio string) {
	base := filepath.Join(gameDir, "Video", strings.ToLower(name))
	return base + ".ivf", base + ".ogg"
}

// IVF is an IVF container: a 32-byte header ("DKIF", version, header size,
// codec FourCC, width, height, rate, scale, frame count) and frames of a
// 12-byte header (size, 64-bit timestamp) plus the payload.
type IVF struct {
	FourCC        string
	Width, Height int
	Rate, Scale   int // frames per second = Rate/Scale
	Frames        [][]byte
}

// ParseIVF splits an IVF file into its frames; the payloads alias data.
func ParseIVF(data []byte) (*IVF, error) {
	if len(data) < 32 || string(data[:4]) != "DKIF" {
		return nil, fmt.Errorf("ivf: bad signature")
	}
	hl := int(binary.LittleEndian.Uint16(data[6:]))
	if hl < 32 || hl > len(data) {
		return nil, fmt.Errorf("ivf: header size %d", hl)
	}
	v := &IVF{
		FourCC: string(data[8:12]),
		Width:  int(binary.LittleEndian.Uint16(data[12:])),
		Height: int(binary.LittleEndian.Uint16(data[14:])),
		Rate:   int(binary.LittleEndian.Uint32(data[16:])),
		Scale:  int(binary.LittleEndian.Uint32(data[20:])),
	}
	if v.Rate <= 0 || v.Scale <= 0 {
		return nil, fmt.Errorf("ivf: frame rate %d/%d", v.Rate, v.Scale)
	}
	for p := hl; p < len(data); {
		if p+12 > len(data) {
			return nil, fmt.Errorf("ivf: truncated frame header at %d", p)
		}
		n := int(binary.LittleEndian.Uint32(data[p:]))
		p += 12
		if n > len(data)-p {
			return nil, fmt.Errorf("ivf: frame of %d bytes at %d past the end", n, p)
		}
		v.Frames = append(v.Frames, data[p:p+n])
		p += n
	}
	return v, nil
}

// FPS is the frame rate.
func (v *IVF) FPS() float64 { return float64(v.Rate) / float64(v.Scale) }

// FilmDecoder decodes an AV1 IVF film frame by frame into RGBA.
type FilmDecoder struct {
	ivf  *IVF
	dec  av1.Decoder
	next int            // next IVF frame (temporal unit) to send
	pend []*av1.Picture // shown pictures not yet returned
}

// NewFilmDecoder opens an IVF stream; the codec must be AV1.
func NewFilmDecoder(data []byte) (*FilmDecoder, error) {
	v, err := ParseIVF(data)
	if err != nil {
		return nil, err
	}
	if v.FourCC != "AV01" {
		return nil, fmt.Errorf("ivf: codec %q, want AV01", v.FourCC)
	}
	return &FilmDecoder{ivf: v, dec: av1.Decoder{FrameThreads: 1}}, nil
}

// IVF returns the container header and frame list.
func (f *FilmDecoder) IVF() *IVF { return f.ivf }

// Next decodes the next shown frame into dst (resized as needed) and
// reports false at the end of the stream.
func (f *FilmDecoder) Next(dst *image.RGBA) (*image.RGBA, bool, error) {
	for len(f.pend) == 0 {
		if f.next >= len(f.ivf.Frames) {
			return dst, false, nil
		}
		pics, err := f.dec.DecodeOBUs(f.ivf.Frames[f.next])
		f.next++
		if err != nil {
			return dst, false, fmt.Errorf("film frame %d: %w", f.next-1, err)
		}
		f.pend = append(f.pend, pics...)
	}
	p := f.pend[0]
	f.pend = f.pend[1:]
	dst, err := YUVToRGBA(dst, p)
	p.Release()
	return dst, err == nil, err
}

// YUVToRGBA converts an 8-bit 4:2:0 picture to RGBA, reusing dst when it has
// the right size. The matrix is BT.709 when the stream says so, else BT.601
// (WMV3's, which the conversion leaves unspecified); limited range unless
// the stream is full range.
func YUVToRGBA(dst *image.RGBA, p *av1.Picture) (*image.RGBA, error) {
	if p.BitDepth != 8 || p.Layout != av1.LayoutI420 {
		return dst, fmt.Errorf("film: %d-bit layout %d, want 8-bit 4:2:0", p.BitDepth, p.Layout)
	}
	w, h := p.Width, p.Height
	if dst == nil || dst.Rect.Dx() != w || dst.Rect.Dy() != h {
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	// Fixed point ×2^16: R = Y' + rv·V, G = Y' − gu·U − gv·V, B = Y' + bu·U.
	m := yuvMatrices[b2i(p.MatrixCoeffs == 1)][b2i(p.FullRange)]
	ys, rv, gu, gv, bu, yoff := m[0], m[1], m[2], m[3], m[4], m[5]
	Y, U, V := p.Data[0], p.Data[1], p.Data[2]
	for y := range h {
		yr := Y[y*p.Stride[0]:]
		ur := U[(y>>1)*p.Stride[1]:]
		vr := V[(y>>1)*p.Stride[2]:]
		o := dst.Pix[y*dst.Stride:]
		for x := range w {
			l := (int32(yr[x]) - yoff) * ys
			u := int32(ur[x>>1]) - 128
			v := int32(vr[x>>1]) - 128
			o[4*x] = clamp8((l + rv*v + 32768) >> 16)
			o[4*x+1] = clamp8((l - gu*u - gv*v + 32768) >> 16)
			o[4*x+2] = clamp8((l + bu*u + 32768) >> 16)
			o[4*x+3] = 0xFF
		}
	}
	return dst, nil
}

// yuvMatrices[bt709][full] = {Y scale, R←V, G←U, G←V, B←U, Y offset}, ×2^16.
var yuvMatrices = [2][2][6]int32{
	{{76309, 104597, 25675, 53279, 132201, 16}, {65536, 91881, 22554, 46802, 116130, 0}},  // BT.601
	{{76309, 117489, 13975, 34925, 138438, 16}, {65536, 103206, 12276, 30679, 121609, 0}}, // BT.709
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func clamp8(v int32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
