// Command res-gallery decodes every image resource in a directory and writes
// a browsable HTML gallery. It recognises sequential Type 5 RGB555 files
// (MENU*R, NEW*R, PLAY*R, ...) and Type 1 interface files (INTR*R, INTO*R)
// drawn with the GRAPH.RES palettes; other files are listed with the reason
// they were skipped. Made for the LAN version's files (in the game folder
// next to the single-player ones), which the sprite catalog does not index.
//
//	go run ./cmd/res-gallery                              # LAN files -> output/site/lan
//	go run ./cmd/res-gallery -files all output/res        # every .RES in -dir
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"konung1.5/internal/resfmt"
	"konung1.5/internal/sitenav"
)

type frame struct {
	ID            int
	Image         string
	Width, Height int
	Offset        string
	Palette       string
	Err           string
}

type resFile struct {
	Name, Kind string
	Size       int
	Frames     []frame
	Failed     int
}

type skipped struct {
	Name   string
	Size   int
	Reason string
}

func main() {
	dir := flag.String("dir", "assets/Konung", "directory with the .RES files")
	files := flag.String("files", strings.Join(lanFiles, ","), `comma-separated file names in -dir, or "all" for every .RES`)
	graphPath := flag.String("graph", "assets/Konung/GRAPH.RES", "GRAPH.RES with the palettes for interface (Type 1) files")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: res-gallery [-dir DIR] [-files A.RES,B.RES|all] [-graph GRAPH.RES] [outdir]  (default outdir output/site/lan)")
		flag.PrintDefaults()
	}
	flag.Parse()
	out := filepath.Join(sitenav.DefaultRoot, "lan")
	if flag.NArg() > 0 {
		out = flag.Arg(0)
	}
	var names []string
	if *files != "all" {
		names = strings.Split(*files, ",")
	}
	if err := run(*dir, names, *graphPath, out); err != nil {
		fmt.Fprintln(os.Stderr, "res-gallery:", err)
		os.Exit(1)
	}
}

// lanFiles are the LAN version's .RES files (docs/konung_lan.md#files).
var lanFiles = []string{
	"INTO640R.RES", "ME800R.RES", "NEW640R.RES", "NEW800R.RES", "PLAY640R.RES", "PLAY800R.RES",
	"MAPS.RES", "Quest_r.res", "SOUNDS_.RES", "SPEECHS_.RES",
}

// run decodes names in dir (nil = every .RES there).
func run(dir string, names []string, graphPath, out string) error {
	if names == nil {
		var err error
		if names, err = resNames(dir); err != nil {
			return err
		}
	}
	var palettes [][256]color.RGBA
	if graph, err := os.ReadFile(graphPath); err == nil {
		if palettes, err = resfmt.Palettes(graph); err != nil {
			return fmt.Errorf("%s: %w", graphPath, err)
		}
	}
	var files []resFile
	var skips []skipped
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		imgDir := filepath.Join(out, strings.ReplaceAll(name, ".", "_"))
		var rf resFile
		var reason string
		switch {
		case len(data) >= 8 && binary.BigEndian.Uint32(data) == 5:
			rf, err = type5(name, data, imgDir)
		case len(data) >= 8 && binary.BigEndian.Uint32(data) == 1:
			var res *resfmt.InterfaceRes
			if res, err = resfmt.ParseInterface(data); err != nil {
				reason = "tag 1 but not an interface file: " + err.Error()
				err = nil
			} else if palettes == nil {
				reason = "interface file; needs " + graphPath + " for its palettes"
			} else {
				rf, err = interfaceRes(name, res, palettes, imgDir)
			}
		default:
			reason = fmt.Sprintf("not an image resource (starts % x)", data[:min(8, len(data))])
		}
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if reason != "" {
			skips = append(skips, skipped{name, len(data), reason})
			continue
		}
		rf.Size = len(data)
		files = append(files, rf)
		fmt.Printf("%-14s %-9s %3d images, %d failed\n", name, rf.Kind, len(rf.Frames)-rf.Failed, rf.Failed)
	}
	return writePage(dir, out, files, skips)
}

// resNames lists the *.RES files of dir, any case, sorted.
func resNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".res") {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToUpper(names[i]) < strings.ToUpper(names[j]) })
	return names, nil
}

func type5(name string, data []byte, imgDir string) (resFile, error) {
	rf := resFile{Name: name, Kind: "Type 5"}
	entries, err := resfmt.Type5Entries(data)
	if err != nil && len(entries) == 0 {
		return rf, err
	}
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		return rf, err
	}
	for i, e := range entries {
		f := frame{ID: i, Offset: fmt.Sprintf("%#06x", e.Offset)}
		img, derr := resfmt.DecodeRLEFrame(e.Data)
		if err := addImage(&rf, &f, img, derr, imgDir); err != nil {
			return rf, err
		}
	}
	if err != nil { // trailing bytes after the last good frame
		rf.Frames = append(rf.Frames, frame{ID: len(entries), Err: err.Error()})
		rf.Failed++
	}
	return rf, nil
}

func interfaceRes(name string, res *resfmt.InterfaceRes, palettes [][256]color.RGBA, imgDir string) (resFile, error) {
	rf := resFile{Name: name, Kind: "Interface"}
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		return rf, err
	}
	for slot := 0; slot < 512; slot++ {
		_, pal, ok := res.Slot(slot)
		if !ok {
			continue
		}
		f := frame{ID: slot, Palette: fmt.Sprintf("%d", pal/0x200)}
		img, derr := res.Image(slot, palettes)
		if err := addImage(&rf, &f, img, derr, imgDir); err != nil {
			return rf, err
		}
	}
	return rf, nil
}

// addImage writes img as a PNG under imgDir and appends the frame, or records
// the decode error.
func addImage(rf *resFile, f *frame, img *image.RGBA, decodeErr error, imgDir string) error {
	if decodeErr != nil {
		f.Err = decodeErr.Error()
		rf.Failed++
		rf.Frames = append(rf.Frames, *f)
		return nil
	}
	file := fmt.Sprintf("%03d.png", f.ID)
	w, err := os.Create(filepath.Join(imgDir, file))
	if err != nil {
		return err
	}
	err = png.Encode(w, img)
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	f.Image = filepath.Base(imgDir) + "/" + file
	f.Width, f.Height = img.Bounds().Dx(), img.Bounds().Dy()
	rf.Frames = append(rf.Frames, *f)
	return nil
}

func writePage(dir, out string, files []resFile, skips []skipped) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	page := struct {
		Dir     string
		Style   template.CSS
		Nav     template.HTML
		Files   []resFile
		Skipped []skipped
	}{Dir: dir, Files: files, Skipped: skips}
	root, inSite := sitenav.SectionRoot(out)
	if inSite {
		page.Style, page.Nav = template.CSS(sitenav.Style), sitenav.Nav("lan")
	}
	f, err := os.Create(filepath.Join(out, "index.html"))
	if err != nil {
		return err
	}
	err = pageTemplate.Execute(f, page)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", filepath.Join(out, "index.html"))
	if inSite {
		return sitenav.WriteHub(root)
	}
	return nil
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>Konung resources: {{.Dir}}</title>
<style>
body{font:14px system-ui;margin:20px;background:#1d2024;color:#eee}
{{.Style}}
a{color:#9cc8ff}.muted{color:#aab2bb}code{background:#25292e;padding:2px 4px;border-radius:3px;font-size:12px}
.toc{display:flex;flex-wrap:wrap;gap:6px;margin:8px 0 16px}.toc a{background:#30353b;padding:4px 8px;border-radius:4px;text-decoration:none}
h2{margin:28px 0 8px;border-bottom:1px solid #3a4047;padding-bottom:4px}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(180px,1fr));gap:10px}
.cell{background:#30353b;border-radius:6px;padding:6px;font-size:12px}
.thumb{display:flex;align-items:center;justify-content:center;height:150px;border-radius:4px;
background:repeating-conic-gradient(#3b4148 0 25%,#2b3036 0 50%) 0 0/16px 16px}
.thumb img{max-width:100%;max-height:150px;image-rendering:pixelated}
.err{color:#ff9c9c}#filter{padding:4px 8px;width:240px}
table{border-collapse:collapse}td,th{padding:4px 10px;border-bottom:1px solid #3a4047;text-align:left}
</style></head><body>
{{.Nav}}
<h1>Konung resources: <code>{{.Dir}}</code></h1>
<p class="muted">Every image decoded from the .RES files in this directory. Type 5 files are RGB555 row-RLE frames
(offset = frame header in the file); interface files are indexed RLE images with a GRAPH.RES palette (ID = slot).
Click an image for full size.</p>
<p><input id="filter" placeholder="Filter files, e.g. PLAY800" oninput="filt(this.value)"></p>
<div class="toc">{{range .Files}}<a href="#{{.Name}}">{{.Name}} ({{len .Frames}})</a>{{end}}</div>
{{range .Files}}<section class="res" data-name="{{.Name}}">
<h2 id="{{.Name}}">{{.Name}} <span class="muted">{{.Kind}}, {{.Size}} bytes, {{len .Frames}} images{{if .Failed}}, <span class="err">{{.Failed}} failed</span>{{end}}</span></h2>
<div class="grid">{{range .Frames}}<div class="cell">
{{if .Image}}<a class="thumb" href="{{.Image}}" target="_blank"><img loading="lazy" src="{{.Image}}" alt="{{.ID}}"></a>
<div><b>#{{.ID}}</b> {{.Width}}×{{.Height}}{{if .Offset}} @{{.Offset}}{{end}}{{if .Palette}} pal {{.Palette}}{{end}}</div>
{{else}}<div><b>#{{.ID}}</b>{{if .Offset}} @{{.Offset}}{{end}}</div><div class="err">{{.Err}}</div>{{end}}
</div>{{end}}</div></section>
{{end}}
{{if .Skipped}}<h2>Not image resources</h2><table><tr><th>File</th><th>Bytes</th><th>Reason</th></tr>
{{range .Skipped}}<tr><td>{{.Name}}</td><td>{{.Size}}</td><td>{{.Reason}}</td></tr>{{end}}</table>{{end}}
<script>function filt(q){q=q.toUpperCase();document.querySelectorAll('.res').forEach(s=>{s.style.display=s.dataset.name.toUpperCase().includes(q)?'':'none'})}</script>
</body></html>
`))
