// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"context"
	"fmt"
	"go/token"
	"testing"

	"golang.org/x/tools/gopls/internal/lsp/source"
	"golang.org/x/tools/gopls/internal/span"
)

func TestParseCache(t *testing.T) {
	ctx := context.Background()
	uri := span.URI("file:///myfile")
	fh := makeFakeFileHandle(uri, []byte("package p\n\nconst _ = \"foo\""))

	var cache parseCache
	pgfs1, _, err := cache.parseFiles(ctx, source.ParseFull, fh)
	if err != nil {
		t.Fatal(err)
	}
	pgf1 := pgfs1[0]
	pgfs2, _, err := cache.parseFiles(ctx, source.ParseFull, fh)
	pgf2 := pgfs2[0]
	if err != nil {
		t.Fatal(err)
	}
	if pgf1.File != pgf2.File {
		t.Errorf("parseFiles(%q): unexpected cache miss on repeated call", uri)
	}

	// Fill up the cache with other files, but don't evict the file above.
	files := []source.FileHandle{fh}
	files = append(files, dummyFileHandles(parseCacheMaxFiles-1)...)
	pgfs3, fset, err := cache.parseFiles(ctx, source.ParseFull, files...)
	pgf3 := pgfs3[0]
	if pgf3.File != pgf1.File {
		t.Errorf("parseFiles(%q, ...): unexpected cache miss", uri)
	}
	if pgf3.Tok == pgf1.Tok {
		t.Errorf("parseFiles(%q, ...): unexpectedly matching token file", uri)
	}
	if pgf3.Tok.Base() != pgf1.Tok.Base() || pgf3.Tok.Size() != pgf1.Tok.Size() {
		t.Errorf("parseFiles(%q, ...): result.Tok has base: %d, size: %d, want (%d, %d)", uri, pgf3.Tok.Base(), pgf3.Tok.Size(), pgf1.Tok.Base(), pgf1.Tok.Size())
	}
	if tok := fset.File(token.Pos(pgf3.Tok.Base())); tok != pgf3.Tok {
		t.Errorf("parseFiles(%q, ...): result.Tok not contained in FileSet", uri)
	}

	// Now overwrite the cache, after which we should get new results.
	files = dummyFileHandles(parseCacheMaxFiles)
	_, _, err = cache.parseFiles(ctx, source.ParseFull, files...)
	if err != nil {
		t.Fatal(err)
	}
	pgfs4, _, err := cache.parseFiles(ctx, source.ParseFull, fh)
	if err != nil {
		t.Fatal(err)
	}
	if pgfs4[0].File == pgf1.File {
		t.Errorf("parseFiles(%q): unexpected cache hit after overwriting cache", uri)
	}
}

func TestParseCache_Reparsing(t *testing.T) {
	defer func(padding int) {
		parsePadding = padding
	}(parsePadding)
	parsePadding = 0

	files := dummyFileHandles(parseCacheMaxFiles)
	danglingSelector := []byte("package p\nfunc _() {\n\tx.\n}")
	files = append(files, makeFakeFileHandle("file:///bad1", danglingSelector))
	files = append(files, makeFakeFileHandle("file:///bad2", danglingSelector))

	// Parsing should succeed even though we overflow the padding.
	var cache parseCache
	_, _, err := cache.parseFiles(context.Background(), source.ParseFull, files...)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseCache_Duplicates(t *testing.T) {
	ctx := context.Background()
	uri := span.URI("file:///myfile")
	fh := makeFakeFileHandle(uri, []byte("package p\n\nconst _ = \"foo\""))

	var cache parseCache
	pgfs, _, err := cache.parseFiles(ctx, source.ParseFull, fh, fh)
	if err != nil {
		t.Fatal(err)
	}
	if pgfs[0].File != pgfs[1].File {
		t.Errorf("parseFiles(fh, fh): = [%p, %p], want duplicate files", pgfs[0].File, pgfs[1].File)
	}
}

func dummyFileHandles(n int) []source.FileHandle {
	var fhs []source.FileHandle
	for i := 0; i < n; i++ {
		uri := span.URI(fmt.Sprintf("file:///_%d", i))
		src := []byte(fmt.Sprintf("package p\nvar _ = %d", i))
		fhs = append(fhs, makeFakeFileHandle(uri, src))
	}
	return fhs
}

func makeFakeFileHandle(uri span.URI, src []byte) fakeFileHandle {
	return fakeFileHandle{
		uri:  uri,
		data: src,
		hash: source.HashOf(src),
	}
}

type fakeFileHandle struct {
	source.FileHandle
	uri  span.URI
	data []byte
	hash source.Hash
}

func (h fakeFileHandle) URI() span.URI {
	return h.uri
}

func (h fakeFileHandle) Read() ([]byte, error) {
	return h.data, nil
}

func (h fakeFileHandle) FileIdentity() source.FileIdentity {
	return source.FileIdentity{
		URI:  h.uri,
		Hash: h.hash,
	}
}
