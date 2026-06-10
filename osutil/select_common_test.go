package osutil

import "testing"

// TestParseSelectionListHandlesNewlinesAndPipes verifies selection parsing accepts
// both newline- and pipe-delimited helper output.
func TestParseSelectionListHandlesNewlinesAndPipes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "newline separated", raw: " /tmp/a\n/tmp/b \n", want: []string{"/tmp/a", "/tmp/b"}},
		{name: "pipe separated", raw: " /tmp/a| /tmp/b |", want: []string{"/tmp/a", "/tmp/b"}},
		{name: "empty", raw: "   ", want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSelectionList(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("len(got) = %d, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
