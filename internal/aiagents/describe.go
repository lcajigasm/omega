package aiagents

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// describe names each session and group after its project folder, and
// fills in what Claude Code's own files say: each open session's ID, status
// and title, and the title of each closed one.
func describe(r *Report, s snap, home string) {
	projects := ""
	if home != "" {
		projects = filepath.Join(home, ".claude", "projects")
	}
	for i := range r.Sessions {
		ss := &r.Sessions[i]
		ss.Project = firstOf(base(ss.Cwd))
		a, _ := ByID(ss.Agent)
		if a == nil || !a.Records || home == "" {
			continue
		}
		rec := readRecord(filepath.Join(home, ".claude", "sessions", strconv.Itoa(ss.PID)+".json"), s.procs[ss.PID])
		if rec == nil {
			continue
		}
		ss.ID, ss.Cwd, ss.Title = rec.SessionID, rec.Cwd, rec.Name
		if rec.Status != "" {
			ss.Status = rec.Status
		}
		if ms := rec.since(); ms > 0 {
			ss.Since = time.UnixMilli(ms)
		}
		info := lookup(projects, ss.ID)
		ss.Project = firstOf(base(ss.Cwd), info.project)
		if ss.Title == "" {
			ss.Title = info.title
		}
	}
	// The open sessions' processes were not known until their IDs were.
	r.attach(r.live)
	sort.SliceStable(r.Sessions, func(i, j int) bool {
		a, b := r.Sessions[i].IdleFor(r.Taken), r.Sessions[j].IdleFor(r.Taken)
		if a != b {
			return a > b
		}
		return r.Sessions[i].Age > r.Sessions[j].Age
	})

	for i := range r.Leftovers {
		g := &r.Leftovers[i]
		var meta info
		if a, _ := ByID(g.Agent); a != nil && a.Records && projects != "" {
			meta = lookup(projects, g.Session)
		}
		g.Title = meta.title
		cwd := ""
		for _, p := range g.Procs {
			if p.Cwd != "" {
				cwd = p.Cwd
				break
			}
		}
		g.Project = firstOf(meta.project, base(cwd))
	}
}

func firstOf(s ...string) string {
	for _, v := range s {
		if v != "" && v != "." && v != "/" {
			return v
		}
	}
	return ""
}

// record is the file Claude Code keeps per running process in
// ~/.claude/sessions/<pid>.json. It is not a documented interface, so every
// field is optional: without it a session still shows, without a status.
type record struct {
	SessionID       string          `json:"sessionId"`
	Cwd             string          `json:"cwd"`
	Name            string          `json:"name"`
	Status          string          `json:"status"`
	ProcStart       string          `json:"procStart"`
	StartedAt       json.Number     `json:"startedAt"`
	StatusUpdatedAt json.RawMessage `json:"statusUpdatedAt"`
}

func (r *record) since() int64 {
	v := strings.Trim(string(r.StatusUpdatedAt), `"`)
	n, _ := strconv.ParseFloat(v, 64)
	return int64(n)
}

// readRecord reads the record of a process, and only if it is about the
// process running now: PIDs get reused.
func readRecord(path string, p *raw) *record {
	if p == nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var rec record
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if dec.Decode(&rec) != nil {
		return nil
	}
	if !rec.sameProcess(p) {
		return nil
	}
	return &rec
}

func (rec *record) sameProcess(p *raw) bool {
	if rec.ProcStart != "" && p.lstart != "" {
		return squeeze(rec.ProcStart) == squeeze(p.lstart)
	}
	// Where the start is recorded in another form, the time the record was
	// made must fall within a minute of the process's start.
	if ms, err := rec.StartedAt.Int64(); err == nil && ms > 0 && !p.start.IsZero() {
		d := time.UnixMilli(ms).Sub(p.start)
		return d > -5*time.Second && d < time.Minute
	}
	return false
}

type info struct{ project, title string }

// lookup reads the project and title of a session from its transcript in
// ~/.claude/projects/<folder>/<id>.jsonl.
func lookup(projects, id string) info {
	if id == "" || id == "no-session" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return info{}
	}
	matches, _ := filepath.Glob(filepath.Join(projects, "*", id+".jsonl"))
	if len(matches) == 0 {
		return info{}
	}
	cwd, title := read(matches[0])
	return info{project: base(cwd), title: title}
}

func base(dir string) string {
	if dir == "" {
		return ""
	}
	return filepath.Base(dir)
}

// read looks in a transcript, which can be hundreds of megabytes, for the
// folder the session started in (near the top: later lines carry the folder
// of the last cd) and the title Claude Code gave it (near the end).
func read(path string) (cwd, title string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	const chunk = 2 << 20
	cwd, _ = scan(io.LimitReader(f, 256<<10), true)
	if st, err := f.Stat(); err == nil && st.Size() > chunk {
		f.Seek(st.Size()-chunk, io.SeekStart)
	} else {
		f.Seek(0, io.SeekStart)
	}
	last, title := scan(f, false)
	if cwd == "" {
		cwd = last
	}
	return cwd, title
}

// scan reads transcript lines for the folder and the title. first stops at
// the first folder.
func scan(r io.Reader, first bool) (cwd, title string) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 2<<20)
	for sc.Scan() {
		line := sc.Bytes()
		hasTitle := bytes.Contains(line, []byte(`"ai-title"`)) || bytes.Contains(line, []byte(`"custom-title"`))
		hasCwd := bytes.Contains(line, []byte(`"cwd"`))
		if !hasTitle && !hasCwd {
			continue
		}
		var v struct {
			Type        string `json:"type"`
			Cwd         string `json:"cwd"`
			AITitle     string `json:"aiTitle"`
			CustomTitle string `json:"customTitle"`
		}
		if json.Unmarshal(line, &v) != nil {
			continue
		}
		if v.Cwd != "" {
			cwd = v.Cwd
			if first {
				return cwd, title
			}
		}
		if v.CustomTitle != "" {
			title = v.CustomTitle
		} else if v.AITitle != "" {
			title = v.AITitle
		}
	}
	return cwd, title
}

// details adds the working folder and the listening ports of each process,
// as lsof reports them. Without lsof they stay empty.
func details(r *Report) {
	var rows []*Proc
	for gi := range r.Leftovers {
		for pi := range r.Leftovers[gi].Procs {
			rows = append(rows, &r.Leftovers[gi].Procs[pi])
		}
	}
	for i := range r.live {
		rows = append(rows, &r.live[i])
	}
	// The agents' own processes, for the folder each session works in.
	sess := make([]Proc, len(r.Sessions))
	for i, s := range r.Sessions {
		sess[i] = Proc{PID: s.PID}
		rows = append(rows, &sess[i])
	}
	defer func() {
		for i := range r.Sessions {
			r.Sessions[i].Cwd = sess[i].Cwd
		}
	}()
	if len(rows) == 0 {
		return
	}
	index := map[int]*Proc{}
	var mains, all []string
	for _, p := range rows {
		index[p.PID] = p
		mains = append(mains, strconv.Itoa(p.PID))
		all = append(all, strconv.Itoa(p.PID))
		for _, h := range p.Helpers {
			index[h.PID] = p
			all = append(all, strconv.Itoa(h.PID))
		}
	}
	lsof, err := exec.LookPath("lsof")
	if err != nil {
		for _, p := range rows {
			p.Cwd = cwdOf(p.PID)
		}
		return
	}
	parse := func(out []byte, each func(p *Proc, name string)) {
		var cur *Proc
		for _, line := range strings.Split(string(out), "\n") {
			switch {
			case strings.HasPrefix(line, "p"):
				cur = index[atoi(line[1:])]
			case strings.HasPrefix(line, "n") && cur != nil:
				each(cur, line[1:])
			}
		}
	}
	out, _ := exec.Command(lsof, "-a", "-d", "cwd", "-p", strings.Join(mains, ","), "-Fpn").Output()
	parse(out, func(p *Proc, name string) { p.Cwd = name })
	out, _ = exec.Command(lsof, "-nP", "-iTCP", "-sTCP:LISTEN", "-a", "-p", strings.Join(all, ","), "-Fpn").Output()
	parse(out, func(p *Proc, name string) {
		port := name[strings.LastIndex(name, ":")+1:]
		for _, have := range p.Ports {
			if have == port {
				return
			}
		}
		p.Ports = append(p.Ports, port)
	})
	for _, p := range rows {
		if p.Cwd == "" {
			p.Cwd = cwdOf(p.PID)
		}
	}
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func selfPID() int { return os.Getpid() }
