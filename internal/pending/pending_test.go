package pending

import (
	"strings"
	"testing"
	"time"
)

func TestNoteSeesAgainAndKeepsDismissed(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	now := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	items, changed, err := Note(nil, "Time Machine", "Time Machine has no backup disk", "backup", now)
	if err != nil || !changed || len(items) != 1 || items[0].ID != "time-machine" {
		t.Fatalf("new item: %+v %v %v", items, changed, err)
	}
	items, changed, _ = Note(items, "time-machine", "Time Machine has no backup disk", "", now.Add(time.Hour))
	if changed || items[0].Seen != 2 {
		t.Fatalf("same item seen again: %+v %v", items, changed)
	}
	items, _ = Set(items, "time-machine", Dismissed, time.Time{})
	items, _, _ = Note(items, "time-machine", "Time Machine has no backup disk", "", now.Add(2*time.Hour))
	if items[0].Status != Dismissed {
		t.Fatalf("a dismissed item must stay dismissed: %+v", items[0])
	}
	s := Summary(items, now)
	if strings.Contains(s, "Known pending") || !strings.Contains(s, "do not mention them: time-machine") {
		t.Fatalf("summary: %q", s)
	}
	if err := Save(items); err != nil {
		t.Fatal(err)
	}
	back, err := Load()
	if err != nil || len(back) != 1 || back[0].Status != Dismissed {
		t.Fatalf("round trip: %+v %v", back, err)
	}
}

func TestSnoozeExpiresAndDoneReopens(t *testing.T) {
	now := time.Now()
	items, _, _ := Note(nil, "docker-prune", "Docker build cache can be pruned", "", now)
	items, _ = Set(items, "docker-prune", Snoozed, now.Add(24*time.Hour))
	if items[0].Active(now) || !items[0].Active(now.Add(25*time.Hour)) {
		t.Fatal("a snooze hides the item until it ends")
	}
	items, _ = Set(items, "docker-prune", Done, time.Time{})
	items, changed, _ := Note(items, "docker-prune", "Docker build cache can be pruned", "", now)
	if !changed || items[0].Status != Open {
		t.Fatalf("an item found again after done is open and new: %+v", items[0])
	}
}

func TestParseFor(t *testing.T) {
	for in, want := range map[string]time.Duration{"3d": 72 * time.Hour, "2w": 14 * 24 * time.Hour, "1m": 30 * 24 * time.Hour, "12h": 12 * time.Hour} {
		if got, err := ParseFor(in); err != nil || got != want {
			t.Errorf("%s: %v %v", in, got, err)
		}
	}
	if _, err := ParseFor("soon"); err == nil {
		t.Error("soon is not a period")
	}
}

func TestBadItemsAreRefused(t *testing.T) {
	if _, _, err := Note(nil, "!!!", "text", "", time.Now()); err == nil {
		t.Error("an id without words")
	}
	items, _, _ := Note(nil, "x", strings.Repeat("a", 1000)+"\x1b[31m", "", time.Now())
	if len(items[0].Text) > maxText || strings.ContainsRune(items[0].Text, 0x1b) {
		t.Errorf("text not cleaned: %q", items[0].Text)
	}
}
