//go:build !js

package gamefs

import "os"

func readFile(name string) ([]byte, error) { return os.ReadFile(name) }

func writeFile(name string, data []byte) error { return os.WriteFile(name, data, 0o644) }

func exists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && !info.IsDir()
}

func readDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
