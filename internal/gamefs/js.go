//go:build js

package gamefs

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall/js"
)

// ManifestFile lists the files served next to the page, one slash-separated
// path per line (written by cmd/web-build).
const ManifestFile = "manifest.txt"

var (
	once     sync.Once
	initErr  error
	mu       sync.Mutex
	served   map[string]bool   // the manifest
	written  map[string][]byte // files written by the game, mirrored in IndexedDB
	jsHelper js.Value          // globalThis.konungFS from konungfs.js
)

func clean(name string) string {
	return strings.TrimPrefix(path.Clean(filepath.ToSlash(name)), "./")
}

func setup() error {
	once.Do(func() {
		served, written = map[string]bool{}, map[string][]byte{}
		jsHelper = js.Global().Get("konungFS")
		if jsHelper.IsUndefined() {
			initErr = errors.New("gamefs: konungFS (konungfs.js) is not loaded")
			return
		}
		text, err := fetch(ManifestFile)
		if err != nil {
			initErr = fmt.Errorf("gamefs: %s: %w", ManifestFile, err)
			return
		}
		for _, line := range strings.Split(string(text), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				served[clean(line)] = true
			}
		}
		// Saves and settings from earlier sessions.
		entries, err := await(jsHelper.Call("load"))
		if err != nil {
			initErr = fmt.Errorf("gamefs: IndexedDB: %w", err)
			return
		}
		for i := 0; i < entries.Length(); i++ {
			e := entries.Index(i)
			written[e.Index(0).String()] = bytesFromJS(e.Index(1))
		}
	})
	return initErr
}

// await blocks the calling goroutine until p settles. It must not be called
// from a JavaScript callback (Ebitengine runs Update in a goroutine).
func await(p js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)
	var then, catch js.Func
	then = js.FuncOf(func(_ js.Value, args []js.Value) any {
		ch <- result{v: args[0]}
		return nil
	})
	catch = js.FuncOf(func(_ js.Value, args []js.Value) any {
		ch <- result{err: errors.New(args[0].Call("toString").String())}
		return nil
	})
	defer then.Release()
	defer catch.Release()
	p.Call("then", then, catch)
	r := <-ch
	return r.v, r.err
}

func bytesFromJS(v js.Value) []byte {
	b := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(b, v)
	return b
}

func fetch(name string) ([]byte, error) {
	v, err := await(jsHelper.Call("fetch", name))
	if err != nil {
		return nil, err
	}
	return bytesFromJS(v), nil
}

func readFile(name string) ([]byte, error) {
	if err := setup(); err != nil {
		return nil, err
	}
	name = clean(name)
	mu.Lock()
	data, ok := written[name]
	mu.Unlock()
	if ok {
		return append([]byte(nil), data...), nil
	}
	if !served[name] {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	data, err := fetch(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return data, nil
}

func writeFile(name string, data []byte) error {
	if err := setup(); err != nil {
		return err
	}
	name = clean(name)
	data = append([]byte(nil), data...)
	mu.Lock()
	written[name] = data
	mu.Unlock()
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	if _, err := await(jsHelper.Call("put", name, arr)); err != nil {
		return &fs.PathError{Op: "write", Path: name, Err: err}
	}
	return nil
}

func exists(name string) bool {
	if setup() != nil {
		return false
	}
	name = clean(name)
	mu.Lock()
	_, ok := written[name]
	mu.Unlock()
	return ok || served[name]
}

func readDir(dir string) ([]string, error) {
	if err := setup(); err != nil {
		return nil, err
	}
	dir = clean(dir)
	seen := map[string]bool{}
	var names []string
	add := func(p string) {
		if d, n := path.Split(p); path.Clean(d) == dir && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	mu.Lock()
	for p := range written {
		add(p)
	}
	mu.Unlock()
	for p := range served {
		add(p)
	}
	if len(names) == 0 {
		return nil, &fs.PathError{Op: "readdir", Path: dir, Err: fs.ErrNotExist}
	}
	return names, nil
}
