package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/assist"
	"github.com/lcajigasm/omega/internal/cleaner"
	"github.com/lcajigasm/omega/internal/gitops"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/ui"
)

func diskCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "disk",
		Short: i18n.T("cmd.disk"),
		Long:  i18n.T("cmd.disk_long"),
		Example: "  omega disk large\n" +
			"  omega disk large ~/Movies --min 500MB --limit 50\n" +
			"  omega disk duplicates ~/Pictures --min 5MB\n" +
			"  omega disk artifacts 60 --dry-run\n" +
			"  omega disk limits",
	}
	c.AddCommand(diskLimitsCmd(), diskDuplicatesCmd(), diskLargeCmd(), diskArtifactsCmd())
	return c
}

// ---- progress on stderr ----

// stderrIsTerminal says whether progress can be drawn. Tests replace it.
var stderrIsTerminal = func() bool { return term.IsTerminal(int(os.Stderr.Fd())) }

// progress is one status line on stderr, rewritten in place and at most
// ten times a second. It is drawn only when stderr is a terminal, so
// pipes, files and --json never see it.
type progress struct {
	mu   sync.Mutex
	w    io.Writer
	on   bool
	last time.Time
	drew bool
}

func newProgress(w io.Writer) *progress {
	return &progress{w: w, on: w == io.Writer(os.Stderr) && stderrIsTerminal()}
}

// Show draws text unless the last line was drawn less than 100 ms ago.
func (p *progress) Show(text string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.on || time.Since(p.last) < 100*time.Millisecond {
		return
	}
	p.draw(text)
}

// Force draws text now.
func (p *progress) Force(text string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.on {
		p.draw(text)
	}
}

func (p *progress) draw(text string) {
	width := 80
	if cols, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil && cols > 10 {
		width = cols
	}
	fmt.Fprint(p.w, "\r\x1b[K"+ansi.Truncate(text, width-1, "…"))
	p.last, p.drew = time.Now(), true
}

// Clear takes the line away.
func (p *progress) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.on && p.drew {
		fmt.Fprint(p.w, "\r\x1b[K")
		p.drew = false
	}
}

// ---- sizes ----

var bareNumber = regexp.MustCompile(`^\s*[0-9]+(?:[.,][0-9]+)?\s*$`)

// parseMinSize reads --min: 500MB, 2GB, 750k; a bare number is megabytes.
func parseMinSize(s string) (int64, error) {
	if bareNumber.MatchString(s) {
		s += "MB"
	}
	n, err := assist.ParseSize(s)
	if err != nil {
		return 0, fmt.Errorf("%s", i18n.T("disk.bad_size", s))
	}
	return n, nil
}

// diskFolder checks the folder to scan: one that is not there or is not a
// folder is a mistake to report, not a reason to scan something else.
func diskFolder(args []string, fallback string) (string, error) {
	dir := fallback
	if len(args) >= 1 {
		dir = args[0]
	}
	fi, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%s", i18n.T("disk.no_folder", dir))
		}
		return "", err
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("%s", i18n.T("disk.not_folder", dir))
	}
	return dir, nil
}

// heavySkips are the folders disk large leaves out unless --all: ~/Library
// (caches and app data that omega's other steps look after) and the cloud
// folders, whose files may not even be on this disk.
func heavySkips() []string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, "Library"),
		filepath.Join(home, "Library", "CloudStorage"),
		filepath.Join(home, "Library", "Mobile Documents"),
	}
}

// scanProgress shows the folders and files seen so far.
func scanProgress(p *progress) func(dirs, files int) {
	return func(dirs, files int) { p.Show(i18n.T("disk.scanning", files, dirs)) }
}

// writeScanNotes says what a scan could not see.
func writeScanNotes(r *ui.Report, st cleaner.ScanStats) {
	if n := len(st.Unreadable); n > 0 {
		shown := make([]string, 0, 3)
		for _, p := range st.Unreadable[:min(3, n)] {
			shown = append(shown, tildePath(p))
		}
		r.Check(ui.MarkWarn, i18n.T("disk.unreadable", n), 0, strings.Join(shown, ", "))
	}
	if len(st.Skipped) > 0 {
		var shown []string
		for _, p := range st.Skipped {
			shown = append(shown, tildePath(p))
		}
		r.Check(ui.MarkSkip, i18n.T("disk.skipped", strings.Join(shown, ", ")), 0, "")
	}
	if st.Interrupted {
		r.Check(ui.MarkWarn, i18n.T("tools.interrupted"), 0, "")
	}
}

// ---- duplicates ----

type dupReport struct {
	Folder     string                   `json:"folder"`
	MinBytes   int64                    `json:"min_bytes"`
	Groups     []cleaner.DuplicateGroup `json:"groups"`
	TotalSets  int                      `json:"total_sets"`
	Wasted     int64                    `json:"wasted"` // bytes, of every set found
	Stats      cleaner.ScanStats        `json:"stats"`
	Truncated  bool                     `json:"truncated,omitempty"`
	Unreadable int                      `json:"unreadable"`
}

func diskDuplicatesCmd() *cobra.Command {
	var minSize string
	var limit int
	var asJSON bool
	c := &cobra.Command{
		Use:     "duplicates [folder]",
		Short:   i18n.T("cmd.disk_dups"),
		Long:    i18n.T("cmd.disk_dups_long"),
		Example: "  omega disk duplicates\n  omega disk duplicates ~/Pictures --min 5MB --limit 50",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, _ := os.UserHomeDir()
			dir, err := diskFolder(args, filepath.Join(home, "Downloads"))
			if err != nil {
				return err
			}
			min, err := parseMinSize(minSize)
			if err != nil {
				return err
			}
			ctx, stop := signalContext()
			defer stop()
			p := newProgress(cmd.ErrOrStderr())
			groups, st, err := cleaner.FindDuplicates(ctx, dir, cleaner.ScanOptions{MinBytes: min, Progress: scanProgress(p)})
			p.Clear()
			if err != nil {
				return err
			}
			rep := dupReport{Folder: dir, MinBytes: min, Groups: groups, TotalSets: len(groups), Stats: st, Unreadable: len(st.Unreadable)}
			for _, g := range groups {
				rep.Wasted += g.Size * int64(len(g.Paths)-1)
			}
			if limit > 0 && len(rep.Groups) > limit {
				rep.Groups, rep.Truncated = rep.Groups[:limit], true
			}
			if rep.Groups == nil {
				rep.Groups = []cleaner.DuplicateGroup{}
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), rep)
			}
			writeDuplicates(cmd.OutOrStdout(), rep)
			return nil
		},
	}
	c.Flags().StringVar(&minSize, "min", "1MB", i18n.T("flag.disk_min"))
	c.Flags().IntVar(&limit, "limit", 20, i18n.T("flag.disk_limit_sets"))
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))
	return c
}

func writeDuplicates(w io.Writer, rep dupReport) {
	r := ui.NewReport(w)
	r.Title("omega disk duplicates", i18n.T("disk.dups_detail", tildePath(rep.Folder), ui.Bytes(rep.MinBytes)))
	r.Blank()
	writeScanNotes(r, rep.Stats)
	if rep.TotalSets == 0 {
		r.Check(ui.MarkOK, i18n.T("disk.no_dups", rep.Stats.Files), 0, "")
		return
	}
	for i, g := range rep.Groups {
		r.Section(i18n.T("disk.dup_set", i+1, ui.Bytes(g.Size), ui.Bytes(g.Size*int64(len(g.Paths)-1))), len(g.Paths))
		for _, path := range g.Paths {
			r.Line(tildePath(path))
		}
	}
	r.Blank()
	r.Line(i18n.T("disk.dups_total", rep.TotalSets, ui.Bytes(rep.Wasted)))
	if rep.Truncated {
		r.Hint(i18n.T("disk.cut", len(rep.Groups), rep.TotalSets))
	}
}

// ---- large ----

type largeReport struct {
	Folder    string              `json:"folder"`
	MinBytes  int64               `json:"min_bytes"`
	Files     []cleaner.LargeFile `json:"files"`
	Total     int                 `json:"total"`
	Truncated bool                `json:"truncated,omitempty"`
	Stats     cleaner.ScanStats   `json:"stats"`
}

func diskLargeCmd() *cobra.Command {
	var minSize string
	var limit int
	var asJSON, all bool
	c := &cobra.Command{
		Use:     "large [folder]",
		Short:   i18n.T("cmd.disk_large"),
		Long:    i18n.T("cmd.disk_large_long"),
		Example: "  omega disk large\n  omega disk large ~/Movies --min 500MB --limit 50\n  omega disk large --all --json",
		// A second argument, the old way to give the size in MB, still works.
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, _ := os.UserHomeDir()
			dir, err := diskFolder(args, home)
			if err != nil {
				return err
			}
			if len(args) == 2 && !cmd.Flags().Changed("min") {
				minSize = args[1]
			}
			min, err := parseMinSize(minSize)
			if err != nil {
				return err
			}
			opts := cleaner.ScanOptions{MinBytes: min}
			if !all {
				opts.Skip = heavySkips()
			}
			ctx, stop := signalContext()
			defer stop()
			p := newProgress(cmd.ErrOrStderr())
			opts.Progress = scanProgress(p)
			files, st, err := cleaner.FindLargeFiles(ctx, dir, opts)
			p.Clear()
			if err != nil {
				return err
			}
			rep := largeReport{Folder: dir, MinBytes: min, Files: files, Total: len(files), Stats: st}
			if limit > 0 && len(files) > limit {
				rep.Files, rep.Truncated = files[:limit], true
			}
			if rep.Files == nil {
				rep.Files = []cleaner.LargeFile{}
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), rep)
			}
			writeLarge(cmd.OutOrStdout(), rep)
			return nil
		},
	}
	c.Flags().StringVar(&minSize, "min", "100MB", i18n.T("flag.disk_min"))
	c.Flags().IntVar(&limit, "limit", 25, i18n.T("flag.disk_limit_files"))
	c.Flags().BoolVar(&all, "all", false, i18n.T("flag.disk_all"))
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))
	return c
}

func writeLarge(w io.Writer, rep largeReport) {
	r := ui.NewReport(w)
	r.Title("omega disk large", i18n.T("disk.large_detail", tildePath(rep.Folder), ui.Bytes(rep.MinBytes)))
	r.Blank()
	writeScanNotes(r, rep.Stats)
	if rep.Total == 0 {
		r.Check(ui.MarkOK, i18n.T("disk.no_large", ui.Bytes(rep.MinBytes), rep.Stats.Files), 0, "")
		return
	}
	var rows [][]string
	var sum int64
	for _, f := range rep.Files {
		rows = append(rows, []string{ui.Bytes(f.Size), tildePath(f.Path)})
		sum += f.Size
	}
	r.Table([]ui.Column{{Title: i18n.T("disk.col_size"), Right: true}, {Title: i18n.T("disk.col_path"), Wrap: true}}, rows, nil)
	r.Blank()
	r.Line(i18n.T("disk.large_total", len(rep.Files), ui.Bytes(sum)))
	if rep.Truncated {
		r.Hint(i18n.T("disk.cut", len(rep.Files), rep.Total))
	}
}

// ---- artifacts ----

type artifactsReport struct {
	Days      int                   `json:"days"`
	Roots     []string              `json:"roots"`
	Artifacts []cleaner.ArtifactDir `json:"artifacts"`
	Total     int64                 `json:"total"` // bytes
}

func diskArtifactsCmd() *cobra.Command {
	var yes, dryRun, asJSON bool
	c := &cobra.Command{
		Use:     "artifacts [days]",
		Short:   i18n.T("cmd.disk_artifacts"),
		Long:    i18n.T("cmd.disk_artifacts_long", strings.Join(gitRootNames(), ", ")),
		Example: "  omega disk artifacts\n  omega disk artifacts 90 --dry-run\n  omega disk artifacts --yes",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			days := 30
			if len(args) == 1 {
				d, err := strconv.Atoi(args[0])
				if err != nil || d <= 0 {
					return fmt.Errorf("%s", i18n.T("disk.bad_days", args[0]))
				}
				days = d
			}
			roots := gitops.DefaultRoots()
			p := newProgress(cmd.ErrOrStderr())
			p.Force(i18n.T("disk.artifacts_scanning"))
			found := cleaner.FindArtifacts(roots, days)
			p.Clear()
			rep := artifactsReport{Days: days, Roots: roots, Artifacts: found}
			if rep.Artifacts == nil {
				rep.Artifacts = []cleaner.ArtifactDir{}
			}
			for _, a := range found {
				rep.Total += a.Size
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), rep)
			}
			w := cmd.OutOrStdout()
			writeArtifacts(w, rep)
			if len(found) == 0 || dryRun {
				return nil
			}
			// The desktop app only lists; deleting is asked for at a
			// terminal, or given with --yes.
			chosen := found
			if !yes {
				if w != io.Writer(os.Stdout) || !stdinIsTerminal() {
					ui.NewReport(w).Hint(i18n.T("disk.artifacts_need_yes"))
					return nil
				}
				var err error
				chosen, err = pickArtifacts(found)
				if err != nil || len(chosen) == 0 {
					fmt.Fprintln(w, i18n.T("tools.nothing_done"))
					return nil
				}
				var size int64
				for _, a := range chosen {
					size += a.Size
				}
				if !askYesNo(i18n.T("disk.artifacts_confirm", len(chosen), ui.Bytes(size))) {
					fmt.Fprintln(w, i18n.T("tools.nothing_done"))
					return nil
				}
			}
			return pruneArtifacts(w, chosen)
		},
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.disk_artifacts_yes"))
	c.Flags().BoolVarP(&dryRun, "dry-run", "n", false, i18n.T("flag.tools_dry"))
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))
	return c
}

func writeArtifacts(w io.Writer, rep artifactsReport) {
	r := ui.NewReport(w)
	r.Title("omega disk artifacts", i18n.T("disk.artifacts_detail", rep.Days))
	r.Blank()
	if len(rep.Artifacts) == 0 {
		r.Check(ui.MarkOK, i18n.T("disk.no_artifacts"), 0, "")
		return
	}
	var rows [][]string
	for _, a := range rep.Artifacts {
		rows = append(rows, []string{ui.Bytes(a.Size), a.Type, i18n.T("disk.days_idle", a.DaysOld), tildePath(a.Path)})
	}
	r.Table([]ui.Column{{Title: i18n.T("disk.col_size"), Right: true}, {Title: i18n.T("disk.col_type")},
		{Title: i18n.T("disk.col_idle"), Right: true}, {Title: i18n.T("disk.col_path"), Wrap: true}}, rows, nil)
	r.Blank()
	r.Line(i18n.T("disk.artifacts_total", len(rep.Artifacts), ui.Bytes(rep.Total)))
}

// pickArtifacts asks which folders to delete, all ticked to start with.
func pickArtifacts(found []cleaner.ArtifactDir) ([]cleaner.ArtifactDir, error) {
	var opts []ui.Option
	for i, a := range found {
		opts = append(opts, ui.Option{ID: strconv.Itoa(i), Label: tildePath(a.Path),
			Detail: a.Type + " · " + i18n.T("disk.days_idle", a.DaysOld), Badge: ui.Bytes(a.Size), Risk: "normal", Selected: true})
	}
	ids, err := ui.Pick(i18n.T("disk.artifacts_pick"), i18n.T("disk.artifacts_pick_hint"), opts, true, true, os.Stdin, os.Stdout)
	if err != nil {
		return nil, err
	}
	var out []cleaner.ArtifactDir
	for _, id := range ids {
		if i, err := strconv.Atoi(id); err == nil && i < len(found) {
			out = append(out, found[i])
		}
	}
	return out, nil
}

func pruneArtifacts(w io.Writer, chosen []cleaner.ArtifactDir) error {
	r := ui.NewReport(w)
	r.Blank()
	failed := 0
	var freed int64
	for _, a := range chosen {
		if err := cleaner.PruneArtifact(a.Path); err != nil {
			failed++
			r.Check(ui.MarkErr, tildePath(a.Path), 0, err.Error())
			continue
		}
		freed += a.Size
		r.Check(ui.MarkOK, tildePath(a.Path), 0, ui.Bytes(a.Size))
	}
	r.Blank()
	r.Line(i18n.T("disk.artifacts_done", len(chosen)-failed, ui.Bytes(freed)))
	if failed > 0 {
		return fmt.Errorf("%s", i18n.T("disk.artifacts_failed", failed, len(chosen)))
	}
	return nil
}
