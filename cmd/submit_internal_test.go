// cmd/submit_internal_test.go
//
// This file uses package cmd (not cmd_test) to unit-test unexported helpers
// like unwrapParagraphs, isBlockElement, etc. that are pure functions with no
// dependency on command wiring. The external test files (package cmd_test)
// cover command-level integration behavior.
package cmd

import (
	"testing"
)

func TestUnwrapParagraphs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "single line",
			in:   "Hello world",
			want: "Hello world",
		},
		{
			name: "hard-wrapped paragraph becomes single line",
			in:   "This is a paragraph that was\nhard-wrapped at around 72 columns\nfor the commit message.",
			want: "This is a paragraph that was hard-wrapped at around 72 columns for the commit message.",
		},
		{
			name: "blank lines preserved as paragraph breaks",
			in:   "First paragraph that is\nhard-wrapped.\n\nSecond paragraph also\nhard-wrapped.",
			want: "First paragraph that is hard-wrapped.\n\nSecond paragraph also hard-wrapped.",
		},
		{
			name: "fenced code block preserved verbatim",
			in:   "Before code:\n\n```go\nfunc main() {\n    fmt.Println(\"hello\")\n}\n```\n\nAfter code.",
			want: "Before code:\n\n```go\nfunc main() {\n    fmt.Println(\"hello\")\n}\n```\n\nAfter code.",
		},
		{
			name: "tilde fenced code block preserved",
			in:   "Text before.\n\n~~~\ncode here\n~~~\n\nText after.",
			want: "Text before.\n\n~~~\ncode here\n~~~\n\nText after.",
		},
		{
			name: "indented code block preserved",
			in:   "Some text.\n\n    indented code line 1\n    indented code line 2\n\nMore text.",
			want: "Some text.\n\n    indented code line 1\n    indented code line 2\n\nMore text.",
		},
		{
			name: "list continuation with indent is joined",
			in:   "Changes:\n\n- First item\n- Second item that is\n  also long\n- Third item",
			want: "Changes:\n\n- First item\n- Second item that is also long\n- Third item",
		},
		{
			name: "list continuation without indent is joined",
			in:   "Changes:\n\n- First item\n- Second item that is\nhard-wrapped here\n- Third item",
			want: "Changes:\n\n- First item\n- Second item that is hard-wrapped here\n- Third item",
		},
		{
			name: "ordered list items preserved",
			in:   "Steps:\n\n1. First step\n2. Second step\n3. Third step",
			want: "Steps:\n\n1. First step\n2. Second step\n3. Third step",
		},
		{
			name: "hard-wrapped ordered list item is joined",
			in:   "Steps:\n\n1. First step that is\nhard-wrapped here\n2. Second step",
			want: "Steps:\n\n1. First step that is hard-wrapped here\n2. Second step",
		},
		{
			name: "nested list items preserved",
			in:   "- Item 1\n  - Nested item\n  - Another nested\n- Item 2",
			want: "- Item 1\n  - Nested item\n  - Another nested\n- Item 2",
		},
		{
			name: "hard-wrapped nested list item is joined",
			in:   "- Item 1\n  - Nested item that is\n    also long\n- Item 2",
			want: "- Item 1\n  - Nested item that is also long\n- Item 2",
		},
		{
			name: "headers preserved",
			in:   "## Section\n\nParagraph that is\nhard-wrapped here.\n\n### Subsection\n\nAnother para.",
			want: "## Section\n\nParagraph that is hard-wrapped here.\n\n### Subsection\n\nAnother para.",
		},
		{
			name: "blockquotes preserved",
			in:   "> This is a quote\n> that spans lines\n\nRegular text.",
			want: "> This is a quote\n> that spans lines\n\nRegular text.",
		},
		{
			name: "horizontal rule preserved",
			in:   "Above\n\n---\n\nBelow",
			want: "Above\n\n---\n\nBelow",
		},
		{
			name: "realistic commit message body",
			in:   "This commit refactors the authentication middleware to\nuse JWT tokens instead of session cookies. The change\nimproves scalability by removing server-side session\nstorage requirements.\n\nKey changes:\n\n- Replace session middleware with JWT validation\n- Add token refresh endpoint\n- Update tests to use new auth flow\n\nBreaking change: clients must now send an\n`Authorization: Bearer <token>` header instead of\nrelying on cookies.",
			want: "This commit refactors the authentication middleware to use JWT tokens instead of session cookies. The change improves scalability by removing server-side session storage requirements.\n\nKey changes:\n\n- Replace session middleware with JWT validation\n- Add token refresh endpoint\n- Update tests to use new auth flow\n\nBreaking change: clients must now send an `Authorization: Bearer <token>` header instead of relying on cookies.",
		},
		{
			name: "pipe tables preserved",
			in:   "Results:\n\n| Name | Value |\n|------|-------|\n| foo  | 42    |",
			want: "Results:\n\n| Name | Value |\n|------|-------|\n| foo  | 42    |",
		},
		{
			name: "trailing whitespace on wrapped lines is trimmed",
			in:   "Line one with trailing space   \nline two.",
			want: "Line one with trailing space line two.",
		},
		{
			name: "HTML tags cause bail-out",
			in:   "Some text that is\nhard-wrapped.\n\n<details>\n<summary>Click me</summary>\nHidden content\n</details>",
			want: "Some text that is\nhard-wrapped.\n\n<details>\n<summary>Click me</summary>\nHidden content\n</details>",
		},
		{
			name: "inline HTML tag causes bail-out",
			in:   "This has a <br/> in it\nand wraps.",
			want: "This has a <br/> in it\nand wraps.",
		},
		{
			name: "angle bracket in non-HTML context still unwraps",
			in:   "The value x < y is\nalways true.",
			want: "The value x < y is always true.",
		},
		{
			name: "HTML inside fenced code block does not trigger bail-out",
			in:   "This adds a component.\n\n```html\n<div class=\"wrapper\">\n  <span>hello</span>\n</div>\n```\n\nThe paragraph is\nhard-wrapped here.",
			want: "This adds a component.\n\n```html\n<div class=\"wrapper\">\n  <span>hello</span>\n</div>\n```\n\nThe paragraph is hard-wrapped here.",
		},
		{
			name: "HTML inside indented code block does not trigger bail-out",
			in:   "Example:\n\n    <div>indented html</div>\n\nMore text that is\nhard-wrapped.",
			want: "Example:\n\n    <div>indented html</div>\n\nMore text that is hard-wrapped.",
		},
		{
			name: "HTML in prose with code block HTML still bails out",
			in:   "Use the <details> tag.\n\n```html\n<div>code</div>\n```",
			want: "Use the <details> tag.\n\n```html\n<div>code</div>\n```",
		},
		{
			name: "mismatched fence markers do not close each other",
			in:   "Text before.\n\n```\n~~~\nstill in code\n```\n\nParagraph that is\nhard-wrapped.",
			want: "Text before.\n\n```\n~~~\nstill in code\n```\n\nParagraph that is hard-wrapped.",
		},
		{
			name: "tilde fence with backticks inside",
			in:   "Text.\n\n~~~\n```\nnested marker\n~~~\n\nWrapped line\ncontinues here.",
			want: "Text.\n\n~~~\n```\nnested marker\n~~~\n\nWrapped line continues here.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unwrapParagraphs(tt.in)
			if got != tt.want {
				t.Errorf("unwrapParagraphs() mismatch\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestContainsHTMLOutsideCode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"no HTML", "just plain text", false},
		{"HTML in prose", "Use <div> here", true},
		{"HTML in fenced code block", "```\n<div>hi</div>\n```", false},
		{"HTML in indented code block", "    <div>hi</div>", false},
		{"HTML in inline code", "Use `<div>` for this", false},
		{"HTML in prose AND code block", "<br/>\n\n```\n<div>x</div>\n```", true},
		{"angle bracket not HTML", "x < y", false},
		{"hyphenated custom element", "Use <my-component> here", true},
		{"namespaced XML tag", "The <xml:tag> element", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsHTMLOutsideCode(tt.in)
			if got != tt.want {
				t.Errorf("containsHTMLOutsideCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsListItem(t *testing.T) {
	listLines := []string{
		"- item",
		"* item",
		"+ item",
		"-",
		"*",
		"+",
		"1. ordered",
		"12. multi-digit",
		"  - indented unordered",
		"  * indented star",
		"  1. indented ordered",
		"\t- tab indented",
	}
	for _, line := range listLines {
		if !isListItem(line) {
			t.Errorf("expected isListItem(%q) = true", line)
		}
	}

	nonListLines := []string{
		"just text",
		"# Header",
		"> blockquote",
		"| table",
		"2nd place finish",
		"",
	}
	for _, line := range nonListLines {
		if isListItem(line) {
			t.Errorf("expected isListItem(%q) = false", line)
		}
	}
}

func TestIsBlockElement(t *testing.T) {
	blockLines := []string{
		"# Header",
		"## Header 2",
		"- list item",
		"* list item",
		"+ list item",
		"1. ordered",
		"12. multi-digit ordered",
		"> blockquote",
		"| table row",
	}
	for _, line := range blockLines {
		if !isBlockElement(line) {
			t.Errorf("expected isBlockElement(%q) = true", line)
		}
	}

	nonBlockLines := []string{
		"just text",
		"This starts a sentence.",
		"2nd place finish",
	}
	for _, line := range nonBlockLines {
		if isBlockElement(line) {
			t.Errorf("expected isBlockElement(%q) = false", line)
		}
	}
}

func TestIsHorizontalRule(t *testing.T) {
	rules := []string{"---", "***", "___", "- - -", "* * *", "----", "****"}
	for _, r := range rules {
		if !isHorizontalRule(r) {
			t.Errorf("expected isHorizontalRule(%q) = true", r)
		}
	}

	nonRules := []string{"--", "**", "-", "abc", "---x"}
	for _, r := range nonRules {
		if isHorizontalRule(r) {
			t.Errorf("expected isHorizontalRule(%q) = false", r)
		}
	}
}
