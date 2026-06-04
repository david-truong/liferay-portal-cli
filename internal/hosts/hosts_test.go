package hosts

import "testing"

func TestUpsertAppendsWhenAbsent(t *testing.T) {
	in := "127.0.0.1\tlocalhost\n255.255.255.255\tbroadcasthost\n"
	out, err := Upsert(in, "lpd-1", "lpd-1-ab12")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	want := in + "127.0.0.1\tlpd-1\t# liferay-cli lpd-1-ab12\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestUpsertReplacesSameID(t *testing.T) {
	in := "127.0.0.1\tlocalhost\n" +
		"127.0.0.1\told-name\t# liferay-cli lpd-1-ab12\n" +
		"127.0.0.1\tother\t# liferay-cli lpd-2-cd34\n"
	out, err := Upsert(in, "new-name", "lpd-1-ab12")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	want := "127.0.0.1\tlocalhost\n" +
		"127.0.0.1\tnew-name\t# liferay-cli lpd-1-ab12\n" +
		"127.0.0.1\tother\t# liferay-cli lpd-2-cd34\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestUpsertIdempotent(t *testing.T) {
	in := "127.0.0.1\tlocalhost\n"
	once, _ := Upsert(in, "lpd-1", "lpd-1-ab12")
	twice, _ := Upsert(once, "lpd-1", "lpd-1-ab12")
	if once != twice {
		t.Fatalf("not idempotent:\n once: %q\ntwice: %q", once, twice)
	}
}

func TestUpsertRejectsBadName(t *testing.T) {
	if _, err := Upsert("", "Bad_Name", "id"); err == nil {
		t.Fatal("expected error for invalid hostname")
	}
}

func TestRemove(t *testing.T) {
	in := "127.0.0.1\tlocalhost\n" +
		"127.0.0.1\tgone\t# liferay-cli lpd-1-ab12\n" +
		"127.0.0.1\tkept\t# liferay-cli lpd-2-cd34\n"
	out, removed := Remove(in, "lpd-1-ab12")
	if !removed {
		t.Fatal("expected removed=true")
	}
	want := "127.0.0.1\tlocalhost\n127.0.0.1\tkept\t# liferay-cli lpd-2-cd34\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestRemoveMissingIsNoop(t *testing.T) {
	in := "127.0.0.1\tlocalhost\n"
	out, removed := Remove(in, "nope")
	if removed {
		t.Fatal("expected removed=false")
	}
	if out != in {
		t.Fatalf("content changed: %q", out)
	}
}

func TestList(t *testing.T) {
	in := "127.0.0.1\tlocalhost\n" +
		"127.0.0.1\tone\t# liferay-cli id-1\n" +
		"# a comment\n" +
		"127.0.0.1\ttwo\t# liferay-cli id-2\n"
	got := List(in)
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(got), got)
	}
	if got[0] != (Entry{Name: "one", ID: "id-1"}) || got[1] != (Entry{Name: "two", ID: "id-2"}) {
		t.Fatalf("unexpected entries: %+v", got)
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"LPD-12345":       "lpd-12345",
		"my_feature/x":    "my-feature-x",
		"--weird--name--": "weird-name",
		"###":             "",
		"Already-Fine":    "already-fine",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateName(t *testing.T) {
	valid := []string{"lpd-1", "lpd-12345.test", "a", "a.b.c"}
	for _, n := range valid {
		if err := ValidateName(n); err != nil {
			t.Errorf("ValidateName(%q) unexpected error: %v", n, err)
		}
	}
	invalid := []string{"", "UPPER", "has space", "-leading", "trailing-", "under_score", "a..b"}
	for _, n := range invalid {
		if err := ValidateName(n); err == nil {
			t.Errorf("ValidateName(%q) expected error", n)
		}
	}
}
