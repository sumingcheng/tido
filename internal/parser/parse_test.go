package parser

import (
	"errors"
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a\r\nb\r\nc", "a\nb\nc"},
		{"a\rb", "a\nb"},
		{"hello   \n\t\n", "hello"},
		{"  keep prefix", "  keep prefix"},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		in   string
		want Format
	}{
		{"", FormatUnknown},
		{"\n  \n\t", FormatUnknown},
		{"plain text\nmore lines", FormatText},
		{"- [ ] task", FormatMarkdown},
		{"intro\n- [x] task", FormatMarkdown},
		{"  - [-] task", FormatMarkdown},
	}
	for _, c := range cases {
		if got := Detect(c.in); got != c.want {
			t.Errorf("Detect(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseMarkdown_Basic(t *testing.T) {
	in := `- [ ] pending task
- [x] done
- [-] doing
- [~] cancelled`
	got, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []Item{
		{Content: "pending task", Status: "pending", Depth: 0},
		{Content: "done", Status: "completed", Depth: 0},
		{Content: "doing", Status: "in_progress", Depth: 0},
		{Content: "cancelled", Status: "cancelled", Depth: 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestParseMarkdown_Nested(t *testing.T) {
	in := `- [ ] parent
  - [-] child A
  - [ ] child B
    - [x] grandchild
- [ ] another root`
	got, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []Item{
		{Content: "parent", Status: "pending", Depth: 0},
		{Content: "child A", Status: "in_progress", Depth: 1},
		{Content: "child B", Status: "pending", Depth: 1},
		{Content: "grandchild", Status: "completed", Depth: 2},
		{Content: "another root", Status: "pending", Depth: 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestParseMarkdown_OddIndentRejected(t *testing.T) {
	in := `- [ ] root
   - [ ] off-by-one indent`
	_, err := Parse(in)
	if err == nil || !errors.Is(err, ErrParse) {
		t.Errorf("want ErrParse, got %v", err)
	}
}

func TestParseMarkdown_IgnoresNonTaskLines(t *testing.T) {
	in := `# 标题
intro paragraph
- [ ] real task
> some quote`
	got, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Content != "real task" {
		t.Errorf("got %+v, want exactly one item 'real task'", got)
	}
}

func TestParseText_Basic(t *testing.T) {
	in := "task one\ntask two\n\ntask three"
	got, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []Item{
		{Content: "task one", Status: "pending", Depth: 0},
		{Content: "task two", Status: "pending", Depth: 0},
		{Content: "task three", Status: "pending", Depth: 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestParse_EmptyError(t *testing.T) {
	cases := []string{"", "  ", "\n\n\t"}
	for _, s := range cases {
		_, err := Parse(s)
		if err == nil {
			t.Errorf("Parse(%q): expected error", s)
		}
	}
}

func TestParseMarkdown_NoTasksError(t *testing.T) {
	in := "- [ ] task" // 这个能解析；测试一个真没 task 的
	_, err := Parse(in)
	if err != nil {
		t.Fatalf("baseline parse failed: %v", err)
	}

	noTasks := "## 标题\n\n纯文本，没 checkbox\n"
	got, err := Parse(noTasks)
	// 这是 text 模式（detect 不到 checkbox），会按文本解析为多条
	if err != nil {
		t.Fatalf("text fallback failed: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("text fallback: got %d items, want 2", len(got))
	}
}
