// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lsp

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/tools/gopls/internal/lsp/protocol"
	"golang.org/x/tools/gopls/internal/lsp/source"
	"golang.org/x/tools/gopls/internal/lsp/source/completion"
	"golang.org/x/tools/gopls/internal/lsp/tests"
	"golang.org/x/tools/gopls/internal/span"
)

func (r *runner) Completion(t *testing.T, src span.Span, test tests.Completion, items tests.CompletionItems) {
	got := r.callCompletion(t, src, func(opts *source.Options) {
		opts.DeepCompletion = false
		opts.Matcher = source.CaseInsensitive
		opts.CompleteUnimported = false
		opts.InsertTextFormat = protocol.SnippetTextFormat
		opts.LiteralCompletions = strings.Contains(string(src.URI()), "literal")
		opts.ExperimentalPostfixCompletions = strings.Contains(string(src.URI()), "postfix")
	})
	got = tests.FilterBuiltins(src, got)
	want := expected(t, test, items)
	if diff := tests.DiffCompletionItems(want, got); diff != "" {
		t.Errorf("mismatching completion items (-want +got):\n%s", diff)
	}
}

func (r *runner) CompletionSnippet(t *testing.T, src span.Span, expected tests.CompletionSnippet, placeholders bool, items tests.CompletionItems) {
	list := r.callCompletion(t, src, func(opts *source.Options) {
		opts.UsePlaceholders = placeholders
		opts.DeepCompletion = true
		opts.Matcher = source.Fuzzy
		opts.CompleteUnimported = false
	})
	got := tests.FindItem(list, *items[expected.CompletionItem])
	want := expected.PlainSnippet
	if placeholders {
		want = expected.PlaceholderSnippet
	}
	if diff := tests.DiffSnippets(want, got); diff != "" {
		t.Errorf("%s", diff)
	}
}

func (r *runner) UnimportedCompletion(t *testing.T, src span.Span, test tests.Completion, items tests.CompletionItems) {
	got := r.callCompletion(t, src, func(opts *source.Options) {})
	got = tests.FilterBuiltins(src, got)
	want := expected(t, test, items)
	if diff := tests.CheckCompletionOrder(want, got, false); diff != "" {
		t.Errorf("%s", diff)
	}
}

func (r *runner) DeepCompletion(t *testing.T, src span.Span, test tests.Completion, items tests.CompletionItems) {
	got := r.callCompletion(t, src, func(opts *source.Options) {
		opts.DeepCompletion = true
		opts.Matcher = source.CaseInsensitive
		opts.CompleteUnimported = false
	})
	got = tests.FilterBuiltins(src, got)
	want := expected(t, test, items)
	if diff := tests.DiffCompletionItems(want, got); diff != "" {
		t.Errorf("mismatching completion items (-want +got):\n%s", diff)
	}
}

func (r *runner) FuzzyCompletion(t *testing.T, src span.Span, test tests.Completion, items tests.CompletionItems) {
	got := r.callCompletion(t, src, func(opts *source.Options) {
		opts.DeepCompletion = true
		opts.Matcher = source.Fuzzy
		opts.CompleteUnimported = false
	})
	got = tests.FilterBuiltins(src, got)
	want := expected(t, test, items)
	if diff := tests.DiffCompletionItems(want, got); diff != "" {
		t.Errorf("mismatching completion items (-want +got):\n%s", diff)
	}
}

func (r *runner) CaseSensitiveCompletion(t *testing.T, src span.Span, test tests.Completion, items tests.CompletionItems) {
	got := r.callCompletion(t, src, func(opts *source.Options) {
		opts.Matcher = source.CaseSensitive
		opts.CompleteUnimported = false
	})
	got = tests.FilterBuiltins(src, got)
	want := expected(t, test, items)
	if diff := tests.DiffCompletionItems(want, got); diff != "" {
		t.Errorf("mismatching completion items (-want +got):\n%s", diff)
	}
}

func (r *runner) RankCompletion(t *testing.T, src span.Span, test tests.Completion, items tests.CompletionItems) {
	got := r.callCompletion(t, src, func(opts *source.Options) {
		opts.DeepCompletion = true
		opts.Matcher = source.Fuzzy
		opts.CompleteUnimported = false
		opts.LiteralCompletions = true
		opts.ExperimentalPostfixCompletions = true
	})
	want := expected(t, test, items)
	if msg := tests.CheckCompletionOrder(want, got, true); msg != "" {
		t.Errorf("%s", msg)
	}
}

func expected(t *testing.T, test tests.Completion, items tests.CompletionItems) []protocol.CompletionItem {
	t.Helper()

	toProtocolCompletionItem := func(item *completion.CompletionItem) protocol.CompletionItem {
		pItem := protocol.CompletionItem{
			Label:  item.Label,
			Kind:   item.Kind,
			Detail: item.Detail,
			Documentation: &protocol.Or_CompletionItem_documentation{
				Value: item.Documentation,
			},
			InsertText: item.InsertText,
			TextEdit: &protocol.TextEdit{
				NewText: item.Snippet(),
			},
			// Negate score so best score has lowest sort text like real API.
			SortText: fmt.Sprint(-item.Score),
		}
		if pItem.InsertText == "" {
			pItem.InsertText = pItem.Label
		}
		return pItem
	}

	var want []protocol.CompletionItem
	for _, pos := range test.CompletionItems {
		want = append(want, toProtocolCompletionItem(items[pos]))
	}
	return want
}

func (r *runner) callCompletion(t *testing.T, src span.Span, options func(*source.Options)) []protocol.CompletionItem {
	t.Helper()

	view, err := r.server.session.ViewOf(src.URI())
	if err != nil {
		t.Fatal(err)
	}
	original := view.Options()
	modified := view.Options().Clone()
	options(modified)
	view, err = r.server.session.SetViewOptions(r.ctx, view, modified)
	if err != nil {
		t.Error(err)
		return nil
	}
	defer r.server.session.SetViewOptions(r.ctx, view, original)

	list, err := r.server.Completion(r.ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: protocol.URIFromSpanURI(src.URI()),
			},
			Position: protocol.Position{
				Line:      uint32(src.Start().Line() - 1),
				Character: uint32(src.Start().Column() - 1),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return list.Items
}
