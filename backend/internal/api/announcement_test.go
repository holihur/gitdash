package api

import "testing"

func TestAnnouncementPayload(t *testing.T) {
	// 标题与正文均为空 → 不启用
	if p := announcementPayload("  ", "", "info"); p["enabled"] != false {
		t.Fatalf("empty announcement should be disabled, got %#v", p)
	}

	// 非法 level 归一为 info
	p := announcementPayload("Hi", "Body", "bogus")
	if p["enabled"] != true || p["level"] != "info" {
		t.Fatalf("bogus level should normalize to info, got %#v", p)
	}
	if p["title"] != "Hi" || p["message"] != "Body" {
		t.Fatalf("title/message mismatch: %#v", p)
	}

	// 合法 level 保留
	if p := announcementPayload("t", "m", "critical"); p["level"] != "critical" {
		t.Fatalf("critical level should be preserved, got %#v", p)
	}

	// 内容指纹：相同内容 id 相同，内容变化 id 变化
	a := announcementPayload("Title", "Body", "warning")
	b := announcementPayload("Title", "Body", "warning")
	c := announcementPayload("Title", "Changed", "warning")
	if a["id"] != b["id"] {
		t.Fatalf("same content should yield same id: %v vs %v", a["id"], b["id"])
	}
	if a["id"] == c["id"] {
		t.Fatalf("changed content should yield a new id")
	}
}
