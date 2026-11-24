package pending

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/i18n"
)

// D7: done items go after 30 days, open items nobody saw turn stale (not
// active), and a run that finds a stale item opens it again.
func TestPruneAgesItems(t *testing.T) {
	now := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	old := now.Add(-31 * 24 * time.Hour)
	items := []Item{
		{ID: "old-done", Text: "x", Status: Done, LastSeen: old, Changed: old},
		{ID: "new-done", Text: "x", Status: Done, LastSeen: old, Changed: now.Add(-time.Hour)},
		{ID: "old-open", Text: "x", Status: Open, LastSeen: old},
		{ID: "new-open", Text: "x", Status: Open, LastSeen: now},
		{ID: "old-dismissed", Text: "x", Status: Dismissed, LastSeen: old},
	}
	got := Prune(items, now)
	var ids []string
	for _, it := range got {
		ids = append(ids, it.ID+"="+it.Status)
	}
	if s := strings.Join(ids, ","); s != "new-done=done,old-open=stale,new-open=open,old-dismissed=dismissed" {
		t.Fatalf("pruned: %s", s)
	}
	if got[1].Active(now) {
		t.Error("a stale item is active")
	}
	if strings.Contains(Summary(got, now), "old-open") {
		t.Error("a stale item is in the instructions")
	}
	got, changed, _ := Note(got, "old-open", "x", "", now)
	if !changed || got[1].Status != Open {
		t.Errorf("seen again: %+v %v", got[1], changed)
	}
}

// D7: the instructions list the 15 items seen last and count the rest.
func TestSummaryIsCapped(t *testing.T) {
	now := time.Now()
	var items []Item
	for i := 0; i < 20; i++ {
		items = append(items, Item{ID: fmt.Sprintf("item-%02d", i), Text: "t", Status: Open, FirstSeen: now, LastSeen: now.Add(time.Duration(i) * time.Minute), Seen: 1})
	}
	s := Summary(items, now)
	if strings.Count(s, "\n- item-") != 15 || !strings.Contains(s, "+5 more") || !strings.Contains(s, "item-19") || strings.Contains(s, "item-04") {
		t.Errorf("summary:\n%s", s)
	}
}

// E6: the errors a person reads are in their language.
func TestErrorsAreTranslated(t *testing.T) {
	i18n.Init("es")
	defer i18n.Init("en")
	if _, err := ParseFor("soon"); err == nil || !strings.Contains(err.Error(), "no es un plazo") {
		t.Errorf("ParseFor: %v", err)
	}
	items := []Item{{ID: "docker-a"}, {ID: "docker-b"}}
	if _, err := Resolve(items, "docker"); err == nil || !strings.Contains(err.Error(), "más de un pendiente") {
		t.Errorf("ambiguous: %v", err)
	}
	if _, err := Resolve(items, "nope"); err == nil || !strings.Contains(err.Error(), "no hay ningún pendiente") {
		t.Errorf("not found: %v", err)
	}
}
