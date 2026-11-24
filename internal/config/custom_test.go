package config

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// Days are read in any of omega's languages, Monday first, without
// repeats; a word that is not a day is refused by name.
func TestParseDays(t *testing.T) {
	for in, want := range map[string][]string{
		"":                {},
		"mon,wed,fri":     {"mon", "wed", "fri"},
		"Fri, mon mon":    {"mon", "fri"},
		"lun,mié,vie":     {"mon", "wed", "fri"},
		"sáb dom":         {"sat", "sun"},
		"mer;jeu":         {"wed", "thu"},
		"Mo, Mi, Fr":      {"mon", "wed", "fri"},
		"seg/qua/sex":     {"mon", "wed", "fri"},
		"Monday,sunday.":  {"mon", "sun"},
		"terça, sábado":   {"tue", "sat"},
		"do, donnerstag":  {"thu"},
		"sab,sam,sa,sat ": {"sat"},
	} {
		got, err := ParseDays(in)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("ParseDays(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	_, err := ParseDays("mon,someday")
	var bad BadDayError
	if !errors.As(err, &bad) || bad.Word != "someday" {
		t.Errorf("a bad day: %v", err)
	}
}

func TestRoutineRunsOn(t *testing.T) {
	wed := time.Date(2026, 11, 25, 10, 0, 0, 0, time.Local) // a Wednesday
	if DayID(wed.Weekday()) != "wed" || DayID(time.Sunday) != "sun" || DayID(time.Monday) != "mon" {
		t.Fatal("DayID")
	}
	if !(Routine{}).RunsOn(wed) || !(Routine{Days: []string{"mon", "wed"}}).RunsOn(wed) || (Routine{Days: []string{"mon"}}).RunsOn(wed) {
		t.Error("RunsOn")
	}
}

// Days are kept in custom.toml as ids; a bad one is refused on saving and
// on reading.
func TestRoutineDaysSavedAndChecked(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := SaveNamedRoutine("weekly", Routine{Steps: []string{"brew"}, Days: []string{"vie", "lun"}}); err != nil {
		t.Fatal(err)
	}
	rs, _, err := LoadRoutines()
	if err != nil || !slices.Equal(rs.Routines["weekly"].Days, []string{"mon", "fri"}) {
		t.Fatalf("%+v %v", rs, err)
	}
	if b, _ := os.ReadFile(CustomPath()); !strings.Contains(string(b), `days = ["mon", "fri"]`) {
		t.Errorf("file:\n%s", b)
	}
	if err := SaveNamedRoutine("weekly", Routine{Steps: []string{"brew"}, Days: []string{"someday"}}); err == nil {
		t.Error("a bad day was saved")
	}
	if err := os.WriteFile(CustomPath(), []byte("[routines.x]\nsteps = [\"brew\"]\ndays = [\"funday\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadRoutines(); err == nil || !strings.Contains(err.Error(), "funday") {
		t.Errorf("a bad day was read: %v", err)
	}
}
