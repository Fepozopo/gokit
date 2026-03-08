package semver

import (
	"testing"
)

func TestParseValid(t *testing.T) {
	cases := []struct {
		in string
		ex string
	}{
		{"1.2.3", "1.2.3"},
		{"v1.2.3", "1.2.3"},
		{"1.2.3-alpha", "1.2.3-alpha"},
		{"1.2.3-alpha.1+build.1", "1.2.3-alpha.1+build.1"},
		{"0.0.1", "0.0.1"},
		{"10.20.30-rc.1", "10.20.30-rc.1"},
	}
	for _, c := range cases {
		v, err := Parse(c.in)
		if err != nil {
			t.Fatalf("Parse(%q) unexpected error: %v", c.in, err)
		}
		if s := v.String(); s != c.ex {
			t.Fatalf("Parse(%q).String() = %q; want %q", c.in, s, c.ex)
		}
	}
}

func TestParseSignatureInBuild(t *testing.T) {
	v, err := Parse("1.2.3+sig.sha256.deadbeef")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if v.Build != "sig.sha256.deadbeef" {
		t.Fatalf("expected Build to be preserved; got %q", v.Build)
	}
	if v.Signature == nil {
		t.Fatalf("expected signature to be parsed")
	}
	if v.Signature.Algo != "sha256" || v.Signature.Hex != "deadbeef" {
		t.Fatalf("unexpected signature parsed: %#v", v.Signature)
	}
	// String() should include the build metadata unchanged
	if s := v.String(); s != "1.2.3+sig.sha256.deadbeef" {
		t.Fatalf("String() = %q; want %q", s, "1.2.3+sig.sha256.deadbeef")
	}

	// also allow sig.<hex> (default algo)
	v2, err := Parse("1.2.3+build.1.sig.abcdef")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if v2.Signature == nil || v2.Signature.Hex != "abcdef" {
		t.Fatalf("expected signature hex abcdef; got %#v", v2.Signature)
	}
}

func TestParseInvalid(t *testing.T) {
	cases := []string{"1.2", "a.b.c", "1.2.x", ""}
	for _, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Fatalf("Parse(%q) expected error", c)
		}
	}
}

func TestEquals(t *testing.T) {
	cases := []struct {
		a    string
		b    string
		want bool
	}{
		{"1.2.3+build1", "1.2.3+build2", true},
		{"1.2.3-alpha.1", "1.2.3-alpha.1", true},
		{"1.2.3", "1.2.4", false},
		{"1.2.3-alpha", "1.2.3", false},
	}
	for _, c := range cases {
		a, err := Parse(c.a)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.a, err)
		}
		b, err := Parse(c.b)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.b, err)
		}
		if a.Equals(b) != c.want {
			t.Fatalf("Equals: %q vs %q = %v; want %v", c.a, c.b, a.Equals(b), c.want)
		}
	}
}

func TestGT(t *testing.T) {
	cases := []struct {
		a    string
		b    string
		want bool // a > b
	}{
		{"1.0.0", "0.9.9", true},
		{"1.2.3", "1.2.2", true},
		{"1.2.3", "1.2.3-alpha", true},
		{"1.2.3-alpha", "1.2.3", false},
		{"1.2.3-alpha", "1.2.3-alpha.1", false},
		{"1.2.3-alpha.1", "1.2.3-alpha", true},
		{"1.0.0-alpha", "1.0.0-1", true}, // non-numeric > numeric
		{"1.0.0-2", "1.0.0-1", true},
		{"1.0.0-alpha.2", "1.0.0-alpha.10", false},
	}
	for _, c := range cases {
		a, err := Parse(c.a)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.a, err)
		}
		b, err := Parse(c.b)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.b, err)
		}
		if a.GT(b) != c.want {
			t.Fatalf("GT: %q > %q = %v; want %v", c.a, c.b, a.GT(b), c.want)
		}
	}
}
