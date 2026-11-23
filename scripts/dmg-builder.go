//go:build ignore

// Build tool only. Compile in the isolated .tools/dmg-builder module.
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/leaanthony/dmg/hfsplus"
	"github.com/leaanthony/dmg/udif"
)

func main() {
	if len(os.Args) != 3 {
		panic("usage: dmg-builder input.zip output.dmg")
	}
	z, err := zip.OpenReader(os.Args[1])
	must(err)
	defer z.Close()
	root := &hfsplus.Entry{Kind: hfsplus.KindDir}
	dirs := map[string]*hfsplus.Entry{"": root}
	var directory func(string) *hfsplus.Entry
	directory = func(name string) *hfsplus.Entry {
		if e := dirs[name]; e != nil {
			return e
		}
		parent := directory(path.Dir(name))
		e := &hfsplus.Entry{Name: path.Base(name), Kind: hfsplus.KindDir, Mode: 0755}
		parent.Children = append(parent.Children, e)
		dirs[name] = e
		return e
	}
	dirs["."] = root
	for _, f := range z.File {
		name := strings.TrimSuffix(f.Name, "/")
		if path.IsAbs(name) || path.Clean(name) != name || strings.Contains(name, "\\") || strings.HasPrefix(name, "../") {
			panic("unsafe archive path")
		}
		// Distribution contains the app and the Applications link only.
		if !strings.HasPrefix(name, "PairShell.app/") && name != "PairShell.app" {
			continue
		}
		if f.FileInfo().IsDir() {
			directory(name)
			continue
		}
		r, err := f.Open()
		must(err)
		data, err := io.ReadAll(r)
		must(err)
		must(r.Close())
		mode := uint16(0644)
		if f.Mode()&0111 != 0 {
			mode = 0755
		}
		parent := directory(path.Dir(name))
		parent.Children = append(parent.Children, &hfsplus.Entry{Name: path.Base(name), Kind: hfsplus.KindFile, Mode: mode, Data: data})
	}
	root.Children = append(root.Children, &hfsplus.Entry{Name: "Applications", Kind: hfsplus.KindSymlink, Target: "/Applications", Mode: 0755})
	volume, err := hfsplus.Build(&hfsplus.Volume{Name: "PairShell", CreatedAt: time.Now(), Children: root.Children})
	must(err)
	out, err := os.OpenFile(os.Args[2], os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	must(err)
	err = udif.Build(out, volume, "whole disk (Apple_HFS : 0)")
	must(err)
	must(out.Close())
	fmt.Println("DMG_READY", os.Args[2])
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
