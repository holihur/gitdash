package api

import "testing"

func TestPatchAddressParts(t *testing.T) {
	owner, repo, ok := patchAddressParts("patches+acme+web@mail.example")
	if !ok || owner != "acme" || repo != "web" {
		t.Fatalf("got %q/%q ok=%v", owner, repo, ok)
	}
	for _, bad := range []string{"patches@mail.example", "reply+abc@mail.example", "patches+acme@mail.example", "patches+a+b+c@x", ""} {
		if _, _, ok := patchAddressParts(bad); ok {
			t.Errorf("%q should not parse as a patch address", bad)
		}
	}
}

func TestLastMessageRef(t *testing.T) {
	if got := lastMessageRef("<a@x> <b@x>\t<c@x>"); got != "<c@x>" {
		t.Fatalf("lastMessageRef = %q", got)
	}
	if got := lastMessageRef(""); got != "" {
		t.Fatalf("empty refs = %q", got)
	}
}

func TestPatchSubjects(t *testing.T) {
	mbox := `From 1111111111111111111111111111111111111111 Mon Sep 17 00:00:00 2001
From: Alice <alice@example.com>
Date: Mon, 1 Jan 2024 00:00:00 +0000
Subject: [PATCH 1/2] add foo

---
 foo.txt | 1 +
 1 file changed, 1 insertion(+)

From 2222222222222222222222222222222222222222 Mon Sep 17 00:00:00 2001
From: Bob <bob@example.com>
Date: Mon, 1 Jan 2024 00:00:00 +0000
Subject: [PATCH v2 2/2] Re: fix bar

---
 bar.txt | 1 +
 1 file changed, 1 insertion(+)
`
	got := patchSubjects(mbox)
	want := []string{"add foo", "fix bar"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("subject[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if s := patchSubjects("no subjects here"); len(s) != 0 {
		t.Fatalf("expected no subjects, got %v", s)
	}
}
