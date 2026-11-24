package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/scheduler"
	"github.com/lcajigasm/omega/internal/ui"
)

func scheduleCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "schedule",
		Short: i18n.T("cmd.schedule"),
		Long:  i18n.T("cmd.schedule_long"),
		Example: "  omega schedule install --interval weekly --task quick --hour 3\n" +
			"  omega schedule status\n" +
			"  omega schedule status --json\n" +
			"  omega schedule uninstall",
	}

	var interval, task string
	var hour, minute int

	install := &cobra.Command{
		Use:   "install",
		Short: i18n.T("cmd.schedule_install"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := scheduler.Install(interval, task, hour, minute); err != nil {
				return err
			}
			r := ui.NewReport(cmd.OutOrStdout())
			r.Check(ui.MarkOK, i18n.T("schedule.installed", intervalWord(interval), fmt.Sprintf("%02d:%02d", hour, minute)), 0, "")
			// Say what will run unattended, and where its output goes.
			r.Detail(i18n.T("schedule.runs", task) + " " + scheduler.Describe(task))
			if next := nextRun(interval, hour, minute, time.Now()); !next.IsZero() {
				r.Detail(i18n.T("schedule.next_at", next.Format("2006-01-02 15:04")))
			}
			if runtime.GOOS == "darwin" {
				r.Detail(i18n.T("schedule.log_in", tildePath(scheduler.LogDir())))
			}
			return nil
		},
	}
	install.Flags().StringVar(&interval, "interval", "weekly", i18n.T("flag.schedule_interval"))
	install.Flags().StringVar(&task, "task", "quick", i18n.T("flag.schedule_task"))
	install.Flags().IntVar(&hour, "hour", 3, i18n.T("flag.schedule_hour"))
	install.Flags().IntVar(&minute, "minute", 0, i18n.T("flag.schedule_minute"))

	var asJSON bool
	status := &cobra.Command{
		Use:   "status",
		Short: i18n.T("cmd.schedule_status"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := scheduler.Status()
			if err != nil {
				return err
			}
			v := describeSchedule(st, runtime.GOOS, time.Now())
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), v)
			}
			writeSchedule(cmd.OutOrStdout(), v)
			return nil
		},
	}
	status.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))

	c.AddCommand(
		install,
		status,
		&cobra.Command{
			Use:   "uninstall",
			Short: i18n.T("cmd.schedule_uninstall"),
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := scheduler.Uninstall(); err != nil {
					return err
				}
				ui.NewReport(cmd.OutOrStdout()).Check(ui.MarkOK, i18n.T("schedule.removed"), 0, "")
				return nil
			},
		},
	)
	return c
}

// scheduleView is what schedule status says.
type scheduleView struct {
	Installed bool      `json:"installed"`
	Platform  string    `json:"platform"`
	Task      string    `json:"task,omitempty"`
	Interval  string    `json:"interval,omitempty"` // daily, weekly, monthly
	Time      string    `json:"time,omitempty"`     // HH:MM
	Next      time.Time `json:"next,omitempty"`
	Job       string    `json:"job,omitempty"` // the job file or task name
	Details   string    `json:"details,omitempty"`
	// LastRun and LastResult come from the system's record of the job
	// (launchd's last exit code, systemd's) and from its log.
	LastRun    time.Time `json:"last_run,omitempty"`
	LastResult string    `json:"last_result,omitempty"`
	LastExit   *int      `json:"last_exit,omitempty"`
	Log        string    `json:"log,omitempty"`
}

// scheduleRun runs a read-only question to the system. Tests replace it.
var scheduleRun = func(command string) runner.Result {
	return runner.Run(context.Background(), command, 5*time.Second, nil)
}

var (
	plistString  = regexp.MustCompile(`<string>([^<]*)</string>`)
	plistInt     = regexp.MustCompile(`<key>(Weekday|Day|Hour|Minute)</key>\s*<integer>(\d+)</integer>`)
	onCalendar   = regexp.MustCompile(`(?m)^OnCalendar=(.*)$`)
	execStart    = regexp.MustCompile(`(?m)^ExecStart="[^"]*" (\S+)`)
	calendarTime = regexp.MustCompile(`(\d{2}):(\d{2})`)
	lastExit     = regexp.MustCompile(`last exit code = (-?\d+)`)
)

// describeSchedule reads the job omega installed: what it runs, when, when
// it runs next and how its last run went.
func describeSchedule(st *scheduler.ScheduleStatus, goos string, now time.Time) scheduleView {
	v := scheduleView{Installed: st.Installed, Platform: st.Platform, Job: st.Target, Details: st.Details,
		Task: st.Task, Interval: st.Interval}
	hour, minute := -1, 0
	switch goos {
	case "darwin":
		if data, err := os.ReadFile(st.Target); err == nil {
			// ProgramArguments: the omega binary, then the task.
			if m := plistString.FindAllStringSubmatch(string(data), -1); len(m) >= 3 {
				v.Task = m[2][1]
			}
			v.Interval = "daily"
			for _, m := range plistInt.FindAllStringSubmatch(string(data), -1) {
				n, _ := strconv.Atoi(m[2])
				switch m[1] {
				case "Weekday":
					v.Interval = "weekly"
				case "Day":
					v.Interval = "monthly"
				case "Hour":
					hour = n
				case "Minute":
					minute = n
				}
			}
		}
		v.Log = filepath.Join(scheduler.LogDir(), "schedule.log")
		if st.Installed {
			res := scheduleRun("launchctl print gui/" + strconv.Itoa(os.Getuid()) + "/com.omega.maintenance")
			if m := lastExit.FindStringSubmatch(res.Output); m != nil {
				n, _ := strconv.Atoi(m[1])
				v.LastExit = &n
			}
		}
		if fi, err := os.Stat(v.Log); err == nil && fi.Size() > 0 {
			v.LastRun = fi.ModTime()
			v.LastResult = lastLine(v.Log)
		}
	case "linux":
		if data, err := os.ReadFile(st.Target); err == nil {
			if m := onCalendar.FindStringSubmatch(string(data)); m != nil {
				cal := m[1]
				switch {
				case strings.HasPrefix(cal, "Sat"):
					v.Interval = "weekly"
				case strings.Contains(cal, "-01 "):
					v.Interval = "monthly"
				default:
					v.Interval = "daily"
				}
				if t := calendarTime.FindStringSubmatch(cal); t != nil {
					hour, _ = strconv.Atoi(t[1])
					minute, _ = strconv.Atoi(t[2])
				}
			}
		}
		service := strings.TrimSuffix(st.Target, ".timer") + ".service"
		if data, err := os.ReadFile(service); err == nil {
			if m := execStart.FindStringSubmatch(string(data)); m != nil {
				v.Task = m[1]
			}
		}
		v.Log = "journalctl --user -u omega-maintenance.service"
		if st.Installed {
			res := scheduleRun("systemctl --user show omega-maintenance.service -p ExecMainExitTimestamp -p ExecMainStatus")
			props := map[string]string{}
			for _, line := range strings.Split(res.Output, "\n") {
				if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
					props[key] = value
				}
			}
			// No exit time: it has not run since the timer was set.
			if t, err := time.Parse("Mon 2006-01-02 15:04:05 MST", props["ExecMainExitTimestamp"]); err == nil {
				v.LastRun = t
				if n, err := strconv.Atoi(props["ExecMainStatus"]); err == nil {
					v.LastExit = &n
				}
			}
		}
	}
	if hour >= 0 {
		v.Time = fmt.Sprintf("%02d:%02d", hour, minute)
		if st.Installed {
			v.Next = nextRun(v.Interval, hour, minute, now)
		}
	}
	return v
}

// lastLine is the last line with text of a file, read from its end.
func lastLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	off := max(0, fi.Size()-4096)
	buf := make([]byte, fi.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(buf)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// nextRun is the next time a schedule runs after now: every day at
// hour:minute, on Saturdays (weekly) or on the 1st (monthly).
func nextRun(interval string, hour, minute int, now time.Time) time.Time {
	at := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	switch interval {
	case "daily":
		if !at.After(now) {
			at = at.AddDate(0, 0, 1)
		}
	case "weekly":
		for at.Weekday() != time.Saturday || !at.After(now) {
			at = at.AddDate(0, 0, 1)
		}
	case "monthly":
		at = time.Date(now.Year(), now.Month(), 1, hour, minute, 0, 0, now.Location())
		if !at.After(now) {
			at = at.AddDate(0, 1, 0)
		}
	default:
		return time.Time{}
	}
	return at
}

// intervalWord is an interval as the user reads it.
func intervalWord(interval string) string {
	return i18n.TOr("schedule.every_"+interval, interval)
}

func writeSchedule(w io.Writer, v scheduleView) {
	r := ui.NewReport(w)
	r.Title("omega schedule", v.Platform)
	r.Blank()
	if !v.Installed {
		r.Check(ui.MarkOff, i18n.T("schedule.not_installed"), 0, "")
		if v.Details != "" {
			r.Detail(v.Details)
		}
		r.Blank()
		r.Hint(i18n.T("schedule.install_hint"))
		return
	}
	const lw = 14
	r.Check(ui.MarkOK, i18n.T("schedule.label_state"), lw, i18n.T("schedule.active"))
	if v.Task != "" {
		r.Check(ui.MarkNone, i18n.T("schedule.label_task"), lw, "omega "+v.Task+" --yes --no-tui --no-ai")
	}
	if v.Interval != "" {
		when := intervalWord(v.Interval)
		if v.Time != "" {
			when += ", " + v.Time
		}
		r.Check(ui.MarkNone, i18n.T("schedule.label_when"), lw, when)
	}
	if !v.Next.IsZero() {
		r.Check(ui.MarkNone, i18n.T("schedule.label_next"), lw, v.Next.Format("2006-01-02 15:04"))
	}
	switch {
	case v.LastExit != nil && *v.LastExit != 0:
		r.Check(ui.MarkErr, i18n.T("schedule.label_last"), lw, i18n.T("schedule.last_failed", *v.LastExit, lastWhen(v.LastRun)))
	case !v.LastRun.IsZero():
		r.Check(ui.MarkOK, i18n.T("schedule.label_last"), lw, i18n.T("schedule.last_ok", lastWhen(v.LastRun)))
	default:
		r.Check(ui.MarkOff, i18n.T("schedule.label_last"), lw, i18n.T("schedule.never"))
	}
	if v.LastResult != "" {
		r.Detail(v.LastResult)
	}
	if v.Log != "" {
		r.Check(ui.MarkNone, i18n.T("schedule.label_log"), lw, tildePath(v.Log))
	}
	if v.Job != "" {
		r.Check(ui.MarkNone, i18n.T("schedule.label_job"), lw, tildePath(v.Job))
	}
	if v.Details != "" && v.Platform == "windows" {
		r.Blank()
		r.Line(r.Muted(v.Details))
	}
	r.Blank()
}

func lastWhen(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04")
}
