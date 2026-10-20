package validation

import (
	"strings"
	"testing"
)

func TestChildName_Accepts(t *testing.T) {
	cases := map[string]string{
		"Aira":                                  "Aira",
		"  Bimo  ":                              "Bimo",
		"Mary-Ann":                              "Mary-Ann",
		"D'Arcy":                                "D'Arcy",
		"Anna Maria":                            "Anna Maria",
		strings.Repeat("A", MaxChildNameLength): strings.Repeat("A", MaxChildNameLength),
	}
	for input, want := range cases {
		got, err := ChildName(input)
		if err != nil {
			t.Errorf("ChildName(%q) ditolak: %v", input, err)
		}
		if got != want {
			t.Errorf("ChildName(%q) = %q, seharusnya %q", input, got, want)
		}
	}
}

func TestChildName_Rejects(t *testing.T) {
	for _, input := range []string{
		"", "   ", "Aira 2", "2Aira", "Aira!", "'Aira", "-Aira", "Aira@home", "Zoë",
		strings.Repeat("A", MaxChildNameLength+1),
	} {
		if _, err := ChildName(input); err == nil {
			t.Errorf("ChildName(%q) seharusnya ditolak", input)
		}
	}
}

func TestEmail(t *testing.T) {
	for _, ok := range []string{"a@b.co", "parent.name+tag@mail.example.com", "x@site.technology", "  a@b.co  "} {
		if _, err := Email(ok); err != nil {
			t.Errorf("Email(%q) ditolak: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "   ", "plain", "a@b", "@b.com", "a@.com", "a b@c.com"} {
		if _, err := Email(bad); err == nil {
			t.Errorf("Email(%q) seharusnya ditolak", bad)
		}
	}
}

func TestEmail_TrimsTheResult(t *testing.T) {
	got, err := Email("  a@b.co  ")
	if err != nil || got != "a@b.co" {
		t.Errorf("Email dirapikan menjadi %q (err=%v)", got, err)
	}
}
