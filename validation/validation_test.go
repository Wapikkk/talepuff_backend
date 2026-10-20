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

func TestFeedbackCategory(t *testing.T) {
	for input, want := range map[string]string{"bug": "bug", " Idea ": "idea", "OTHER": "other"} {
		if got, err := FeedbackCategory(input); err != nil || got != want {
			t.Errorf("FeedbackCategory(%q) = %q, %v", input, got, err)
		}
	}
	for _, bad := range []string{"", "complaint", "bugs", "  "} {
		if _, err := FeedbackCategory(bad); err == nil {
			t.Errorf("FeedbackCategory(%q) seharusnya ditolak", bad)
		}
	}
}

func TestFeedbackMessage(t *testing.T) {
	if got, err := FeedbackMessage("  hello world  "); err != nil || got != "hello world" {
		t.Errorf("dirapikan menjadi %q, %v", got, err)
	}
	if _, err := FeedbackMessage("line one\nline two\tok"); err != nil {
		t.Errorf("baris baru dan tab seharusnya boleh: %v", err)
	}
	if _, err := FeedbackMessage(strings.Repeat("é", MaxFeedbackLength)); err != nil {
		t.Errorf("1000 karakter (bukan byte) seharusnya boleh: %v", err)
	}
	for _, bad := range []string{"", "    ", "hi", strings.Repeat("a", MaxFeedbackLength+1), "bad\x00char", "esc\x1b[31m"} {
		if _, err := FeedbackMessage(bad); err == nil {
			t.Errorf("FeedbackMessage(%q) seharusnya ditolak", bad)
		}
	}
}

func TestAppVersion(t *testing.T) {
	for input, want := range map[string]string{"1.0.0+1": "1.0.0+1", " 2.3-beta ": "2.3-beta", "": "", "<script>": "", "a b": "", strings.Repeat("1", 33): ""} {
		if got := AppVersion(input); got != want {
			t.Errorf("AppVersion(%q) = %q, seharusnya %q", input, got, want)
		}
	}
}

func TestMascot(t *testing.T) {
	for _, ok := range Mascots {
		if got, err := Mascot(ok); err != nil || got != ok {
			t.Errorf("Mascot(%q) = %q, %v", ok, got, err)
		}
	}
	if got, err := Mascot("  Badak  "); err != nil || got != "Badak" {
		t.Errorf("dirapikan menjadi %q, %v", got, err)
	}
	for _, bad := range []string{"", "  ", "badak", "BADAK", "Naga", "Harimau,Badak"} {
		if _, err := Mascot(bad); err == nil {
			t.Errorf("Mascot(%q) seharusnya ditolak", bad)
		}
	}
	if DefaultMascot != "Harimau" {
		t.Errorf("maskot bawaan harus sama dengan default di kolom database")
	}
}
