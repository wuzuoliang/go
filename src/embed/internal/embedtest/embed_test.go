// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package embedtest

import (
	"embed"
	"reflect"
	"testing"
	"testing/fstest"
)

//go:embed testdata/h*.txt
//go:embed c*.txt testdata/g*.txt
var global embed.FS

//go:embed c*txt
var concurrency string

//go:embed testdata/g*.txt
var glass []byte

func testFiles(t *testing.T, f embed.FS, name, data string) {
	t.Helper()
	d, err := f.ReadFile(name)
	if err != nil {
		t.Error(err)
		return
	}
	if string(d) != data {
		t.Errorf("read %v = %q, want %q", name, d, data)
	}
}

func testString(t *testing.T, s, name, data string) {
	t.Helper()
	if s != data {
		t.Errorf("%v = %q, want %q", name, s, data)
	}
}

func testDir(t *testing.T, f embed.FS, name string, expect ...string) {
	t.Helper()
	dirs, err := f.ReadDir(name)
	if err != nil {
		t.Error(err)
		return
	}
	var names []string
	for _, d := range dirs {
		name := d.Name()
		if d.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	if !reflect.DeepEqual(names, expect) {
		t.Errorf("readdir %v = %v, want %v", name, names, expect)
	}
}

// Tests for issue 49514.
var _ = '"'
var _ = '\''
var _ = '🦆'

func TestGlobal(t *testing.T) {
	testFiles(t, global, "concurrency.txt", "Concurrency is not parallelism.\n")
	testFiles(t, global, "testdata/hello.txt", "hello, world\n")
	testFiles(t, global, "testdata/glass.txt", "I can eat glass and it doesn't hurt me.\n")

	if err := fstest.TestFS(global, "concurrency.txt", "testdata/hello.txt"); err != nil {
		t.Fatal(err)
	}

	testString(t, concurrency, "concurrency", "Concurrency is not parallelism.\n")
	testString(t, string(glass), "glass", "I can eat glass and it doesn't hurt me.\n")
}

//go:embed testdata
var testDirAll embed.FS

func TestDir(t *testing.T) {
	all := testDirAll
	testFiles(t, all, "testdata/hello.txt", "hello, world\n")
	testFiles(t, all, "testdata/i/i18n.txt", "internationalization\n")
	testFiles(t, all, "testdata/i/j/k/k8s.txt", "kubernetes\n")
	testFiles(t, all, "testdata/ken.txt", "If a program is too slow, it must have a loop.\n")

	testDir(t, all, ".", "testdata/")
	testDir(t, all, "testdata/i", "i18n.txt", "j/")
	testDir(t, all, "testdata/i/j", "k/")
	testDir(t, all, "testdata/i/j/k", "k8s.txt")
}

var (
	//go:embed testdata
	testHiddenDir embed.FS

	//go:embed testdata/*
	testHiddenStar embed.FS
)

func TestHidden(t *testing.T) {
	dir := testHiddenDir
	star := testHiddenStar

	t.Logf("//go:embed testdata")

	testDir(t, dir, "testdata",
		"-not-hidden/", "ascii.txt", "glass.txt", "hello.txt", "i/", "ken.txt")

	t.Logf("//go:embed testdata/*")

	testDir(t, star, "testdata",
		"-not-hidden/", ".hidden/", "_hidden/", "ascii.txt", "glass.txt", "hello.txt", "i/", "ken.txt")

	testDir(t, star, "testdata/.hidden",
		"fortune.txt", "more/") // but not .more or _more
}

func TestUninitialized(t *testing.T) {
	var uninitialized embed.FS
	testDir(t, uninitialized, ".")
	f, err := uninitialized.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !fi.IsDir() {
		t.Errorf("in uninitialized embed.FS, . is not a directory")
	}
}

var (
	//go:embed "testdata/hello.txt"
	helloT []T
	//go:embed "testdata/hello.txt"
	helloUint8 []uint8
	//go:embed "testdata/hello.txt"
	helloEUint8 []EmbedUint8
	//go:embed "testdata/hello.txt"
	helloBytes EmbedBytes
	//go:embed "testdata/hello.txt"
	helloString EmbedString
)

type T byte
type EmbedUint8 uint8
type EmbedBytes []byte
type EmbedString string

// golang.org/issue/47735
func TestAliases(t *testing.T) {
	all := testDirAll
	want, e := all.ReadFile("testdata/hello.txt")
	if e != nil {
		t.Fatal("ReadFile:", e)
	}
<<<<<<< HEAD
	check := func(g any) {
=======
	check := func(g interface{}) {
>>>>>>> 346b18ee9d15410ab08dd583787c64dbed0666d2
		got := reflect.ValueOf(g)
		for i := 0; i < got.Len(); i++ {
			if byte(got.Index(i).Uint()) != want[i] {
				t.Fatalf("got %v want %v", got.Bytes(), want)
			}
		}
	}
	check(helloT)
	check(helloUint8)
	check(helloEUint8)
	check(helloBytes)
	check(helloString)
}
