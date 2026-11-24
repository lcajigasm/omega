package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// DefaultRoutine is the name of the routine `omega custom` runs when it is
// given none, until the user picks another as the default.
const DefaultRoutine = "default"

// Routine is one of the user's own maintenance runs: the steps `omega
// custom` runs, in the order the user put them. The routines live in
// custom.toml beside config.toml, so a dotfiles repository can carry them to
// other machines; steps a machine lacks are passed over there.
type Routine struct {
	Steps []string `toml:"steps" json:"steps"`
	// StopOnFail lists the steps whose failure ends the run: the steps
	// after one of them do not start.
	StopOnFail []string `toml:"stop_on_fail,omitempty" json:"stop_on_fail,omitempty"`
	// OnlyBelowFreeGB, when set, runs the routine only while the free
	// space of the system disk is under this many GB.
	OnlyBelowFreeGB float64 `toml:"only_below_free_gb,omitempty" json:"only_below_free_gb,omitempty"`
	// Days, when set, are the weekdays (mon … sun) a scheduled run of the
	// routine runs on; on the other days it has nothing to do. Empty: every
	// day.
	Days []string `toml:"days,omitempty" json:"days,omitempty"`
	// Updated is when the routine was last saved.
	Updated time.Time `toml:"updated,omitempty" json:"updated,omitempty"`
}

// Stops reports whether a failure of the step ends the run.
func (r Routine) Stops(id string) bool { return slices.Contains(r.StopOnFail, id) }

// RunsOn reports whether the routine runs on the day of t: always when it
// names no days.
func (r Routine) RunsOn(t time.Time) bool {
	return len(r.Days) == 0 || slices.Contains(r.Days, DayID(t.Weekday()))
}

// Weekdays are the values of `days`, Monday first.
var Weekdays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// DayID is the value of `days` for a weekday.
func DayID(w time.Weekday) string { return Weekdays[(int(w)+6)%7] }

// dayWords are the names ParseDays takes for each weekday, short and long,
// in the languages omega speaks: English, Spanish, French, German and
// Portuguese. The same word never names two days.
var dayWords = map[string]string{}

func init() {
	for id, words := range map[string]string{
		"mon": "mon monday lun lunes lundi mo montag seg segunda segunda-feira",
		"tue": "tue tues tuesday mar martes mardi di dienstag ter terça terca terça-feira terca-feira",
		"wed": "wed wednesday mié mie miércoles miercoles mer mercredi mi mittwoch qua quarta quarta-feira",
		"thu": "thu thur thurs thursday jue jueves jeu jeudi do donnerstag qui quinta quinta-feira",
		"fri": "fri friday vie viernes ven vendredi fr freitag sex sexta sexta-feira",
		"sat": "sat saturday sáb sab sábado sabado sam samedi sa samstag",
		"sun": "sun sunday dom domingo dim dimanche so sonntag",
	} {
		for _, w := range strings.Fields(words) {
			dayWords[w] = id
		}
	}
}

// BadDayError is a word ParseDays does not take for a day.
type BadDayError struct{ Word string }

func (e BadDayError) Error() string {
	return fmt.Sprintf("%q is not a day: use mon, tue, wed, thu, fri, sat, sun", e.Word)
}

// ParseDays reads a list of weekdays, "mon,wed,fri" or "lun mié vie", in
// any of omega's languages, and returns their ids Monday first without
// repeats. "" is no days: every day.
func ParseDays(s string) ([]string, error) {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '/'
	})
	seen := map[string]bool{}
	for _, w := range words {
		w = strings.TrimSuffix(w, ".")
		id, ok := dayWords[w]
		if !ok {
			return nil, BadDayError{w}
		}
		seen[id] = true
	}
	var out []string
	for _, id := range Weekdays {
		if seen[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

// Routines is everything custom.toml holds.
type Routines struct {
	// Default names the routine `omega custom` runs with no name.
	Default  string             `toml:"default,omitempty" json:"default"`
	Routines map[string]Routine `toml:"routines" json:"routines"`
}

// file is custom.toml as written: the routines, and the single list of
// steps of the files written before routines had names.
type customFile struct {
	Routines
	Steps   []string  `toml:"steps,omitempty"`
	Updated time.Time `toml:"updated,omitempty"`
}

// CustomPath is the file that holds the routines.
func CustomPath() string { return filepath.Join(Dir(), "custom.toml") }

// routineName is what a routine may be called: a word that works on a
// command line and as a TOML key.
var routineName = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}_-]{0,39}$`)

// ValidRoutineName reports whether name can name a routine.
func ValidRoutineName(name string) bool { return routineName.MatchString(name) }

// LoadRoutines reads custom.toml. ok is false when there is none. A file
// from before routines had names holds one list of steps: it is read as the
// routine "default".
func LoadRoutines() (rs Routines, ok bool, err error) {
	b, err := os.ReadFile(CustomPath())
	if errors.Is(err, os.ErrNotExist) {
		return Routines{Routines: map[string]Routine{}}, false, nil
	}
	if err != nil {
		return Routines{}, false, err
	}
	var f customFile
	if _, err := toml.Decode(string(b), &f); err != nil {
		return Routines{}, false, fmt.Errorf("%s: %w", CustomPath(), err)
	}
	rs = f.Routines
	if rs.Routines == nil {
		rs.Routines = map[string]Routine{}
	}
	if len(f.Steps) > 0 {
		if _, taken := rs.Routines[DefaultRoutine]; !taken {
			rs.Routines[DefaultRoutine] = Routine{Steps: f.Steps, Updated: f.Updated}
		}
	}
	for name, r := range rs.Routines {
		r.Steps = cleanIDs(r.Steps)
		r.StopOnFail = cleanIDs(r.StopOnFail)
		days, err := ParseDays(strings.Join(r.Days, ","))
		if err != nil {
			return Routines{}, false, fmt.Errorf("%s: routine %s: days: %w", CustomPath(), name, err)
		}
		r.Days = days
		rs.Routines[name] = r
	}
	return rs, true, nil
}

// DefaultName is the routine run when none is named: the one chosen as the
// default, else "default", else the only one there is.
func (rs Routines) DefaultName() string {
	if _, ok := rs.Routines[rs.Default]; ok && rs.Default != "" {
		return rs.Default
	}
	if _, ok := rs.Routines[DefaultRoutine]; ok {
		return DefaultRoutine
	}
	if names := rs.Names(); len(names) == 1 {
		return names[0]
	}
	return DefaultRoutine
}

// Get returns a routine by name; "" is the default one.
func (rs Routines) Get(name string) (Routine, bool) {
	if name == "" {
		name = rs.DefaultName()
	}
	r, ok := rs.Routines[name]
	return r, ok
}

// Names lists the routines in alphabetical order.
func (rs Routines) Names() []string {
	var out []string
	for n := range rs.Routines {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// SaveRoutines writes custom.toml, replacing the file in one step so an
// interrupted write never leaves half a routine behind. With no routines
// left the file is removed.
func SaveRoutines(rs Routines) error {
	for name, r := range rs.Routines {
		if !ValidRoutineName(name) {
			return fmt.Errorf("%q cannot name a routine: use letters, digits, - and _", name)
		}
		r.Steps = cleanIDs(r.Steps)
		if len(r.Steps) == 0 {
			return fmt.Errorf("routine %s: a routine needs at least one step", name)
		}
		r.StopOnFail = cleanIDs(r.StopOnFail)
		if r.OnlyBelowFreeGB < 0 {
			return fmt.Errorf("routine %s: only_below_free_gb cannot be negative", name)
		}
		days, err := ParseDays(strings.Join(r.Days, ","))
		if err != nil {
			return fmt.Errorf("routine %s: days: %w", name, err)
		}
		r.Days = days
		if r.Updated.IsZero() {
			r.Updated = time.Now()
		}
		rs.Routines[name] = r
	}
	if _, ok := rs.Routines[rs.Default]; !ok {
		rs.Default = ""
	}
	if len(rs.Routines) == 0 {
		err := os.Remove(CustomPath())
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var body bytes.Buffer
	body.WriteString("# omega custom: your routines, the steps each one runs, in this order.\n" +
		"# Change them with `omega custom --set [name]`; `omega steps` lists every id.\n" +
		"# stop_on_fail: the steps whose failure ends the run.\n" +
		"# only_below_free_gb: run the routine only when the disk has less free space.\n" +
		"# days: the weekdays (mon … sun) a scheduled run runs it on; none: every day.\n\n")
	if err := toml.NewEncoder(&body).Encode(rs); err != nil {
		return err
	}
	return writeAtomic(CustomPath(), body.Bytes())
}

// LoadRoutine reads the default routine. ok is false when there is none.
func LoadRoutine() (r Routine, ok bool, err error) {
	rs, _, err := LoadRoutines()
	if err != nil {
		return Routine{}, false, err
	}
	r, ok = rs.Get("")
	return r, ok, nil
}

// SaveRoutine saves r as the default routine, keeping the others.
func SaveRoutine(r Routine) error {
	return SaveNamedRoutine("", r)
}

// SaveNamedRoutine saves one routine, keeping the others; "" is the
// default one.
func SaveNamedRoutine(name string, r Routine) error {
	rs, _, err := LoadRoutines()
	if err != nil {
		return err
	}
	if name == "" {
		name = rs.DefaultName()
	}
	if len(cleanIDs(r.Steps)) == 0 {
		return errors.New("a routine needs at least one step")
	}
	rs.Routines[name] = r
	return SaveRoutines(rs)
}

// writeAtomic replaces a file in one step.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// cleanIDs trims the ids and drops empty ones and repeats, keeping the
// first place of each.
func cleanIDs(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
