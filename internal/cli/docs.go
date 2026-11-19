package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/docs/manual"
	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
)

// omega docs: the manual inside the binary. It lists the pages, shows one,
// searches them offline, and answers a question in the user's language with
// the configured model, from the manual's sections and the command tree.

type section struct {
	page    string // file name without .md
	title   string // the page's title
	heading string
	body    string
}

func manualPages() map[string]string {
	pages := map[string]string{}
	fs.WalkDir(manual.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		b, err := manual.FS.ReadFile(p)
		if err == nil {
			pages[strings.TrimSuffix(p, ".md")] = string(b)
		}
		return nil
	})
	return pages
}

var headingRe = regexp.MustCompile(`(?m)^(#{1,3}) (.+)$`)

// manualSections splits every page at its headings.
func manualSections() []section {
	var out []section
	pages := manualPages()
	names := make([]string, 0, len(pages))
	for n := range pages {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		text := pages[name]
		title := name
		if m := headingRe.FindStringSubmatch(text); m != nil {
			title = m[2]
		}
		idx := headingRe.FindAllStringSubmatchIndex(text, -1)
		for i, m := range idx {
			end := len(text)
			if i+1 < len(idx) {
				end = idx[i+1][0]
			}
			body := strings.TrimSpace(text[m[1]:end])
			if body == "" {
				continue
			}
			out = append(out, section{page: name, title: title, heading: text[m[4]:m[5]], body: body})
		}
	}
	return out
}

// synonyms let a question in another language find the English manual.
var synonyms = map[string]string{
	"disco": "disk", "límite": "limit", "limite": "limit", "límites": "limits", "limites": "limits", "limitar": "limit",
	"caché": "cache", "cache": "cache", "borrar": "delete", "eliminar": "remove", "limpiar": "clean", "limpieza": "clean",
	"programar": "schedule", "programada": "schedule", "clave": "key", "modelo": "model", "modelos": "models",
	"actualizar": "update", "permisos": "permissions", "agentes": "agents", "agente": "agent", "asistente": "assist",
	"asistentes": "assist", "configurar": "configure", "configuración": "configuration", "espacio": "space",
	"registros": "logs", "seguridad": "safety", "ayuda": "help", "instalar": "install", "aprobar": "approve",
	"festplatte": "disk", "disque": "disk", "nettoyer": "clean", "espace": "space", "speicher": "disk",
}

var stopwords = map[string]bool{"the": true, "and": true, "how": true, "what": true, "can": true, "does": true, "with": true,
	"for": true, "que": true, "como": true, "cómo": true, "para": true, "una": true, "uno": true, "los": true, "las": true,
	"del": true, "por": true, "con": true, "puedo": true, "omega": true, "this": true, "that": true, "qué": true}

func queryTerms(q string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127 || r == '-')
	}) {
		if len([]rune(w)) < 3 || stopwords[w] {
			continue
		}
		if s, ok := synonyms[w]; ok {
			w = s
		}
		out = append(out, w)
	}
	return out
}

// searchManual returns the sections that best match a query.
func searchManual(q string, n int) []section {
	terms := queryTerms(q)
	if len(terms) == 0 {
		return nil
	}
	type scored struct {
		s     section
		score int
	}
	var hits []scored
	for _, s := range manualSections() {
		head := strings.ToLower(s.heading)
		body := strings.ToLower(s.body)
		score, matched := 0, 0
		for _, t := range terms {
			h := strings.Count(head, t)
			b := strings.Count(body, t)
			if h+b > 0 {
				matched++
			}
			score += 5*h + min(b, 8)
		}
		if score > 0 {
			// Sections that hold every word come first.
			hits = append(hits, scored{s, score + 10*matched})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	var out []section
	for i := 0; i < len(hits) && i < n; i++ {
		out = append(out, hits[i].s)
	}
	return out
}

// manualAnswerContext is the best sections for a query as Markdown.
func manualAnswerContext(q string, n int) string {
	hits := searchManual(q, n)
	if len(hits) == 0 {
		return "Nothing in omega's manual matches that. `omega --help` lists every command."
	}
	var b strings.Builder
	for _, s := range hits {
		fmt.Fprintf(&b, "## %s › %s\n(docs/manual/%s.md)\n\n%s\n\n", s.title, s.heading, s.page, trimBody(s.body, 4000))
	}
	return strings.TrimSpace(b.String())
}

func trimBody(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…"
}

// commandSummary is every command with its one-line help.
func commandSummary(c *cobra.Command) string {
	var b strings.Builder
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Hidden || sub.Name() == "help" {
				continue
			}
			fmt.Fprintf(&b, "%s — %s\n", sub.CommandPath(), sub.Short)
			walk(sub)
		}
	}
	walk(c)
	return b.String()
}

func docsCmd(root *cobra.Command) *cobra.Command {
	c := &cobra.Command{
		Use:     "docs [page]",
		Aliases: []string{"manual", "doc", "guide"},
		Short:   i18n.T("cmd.docs"),
		Long: "The manual is inside omega. Without arguments it lists the pages; with a page name it shows it.\n\n" +
			"  omega docs                          the pages\n" +
			"  omega docs tools                    one page\n" +
			"  omega docs search docker disk       the sections that match, offline\n" +
			"  omega docs ask \"how do I stop Docker filling my disk?\"\n\n" +
			"ask answers in your language with your configured model, from the manual and the command list\n" +
			"only, and says the commands to type. Without a key it shows the matching sections instead.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			pages := manualPages()
			if len(args) == 0 {
				var names []string
				for n := range pages {
					names = append(names, n)
				}
				sort.Strings(names)
				fmt.Fprintln(w, "\nOMEGA MANUAL")
				for _, n := range names {
					title := n
					if m := headingRe.FindStringSubmatch(pages[n]); m != nil {
						title = m[2]
					}
					fmt.Fprintf(w, "  %-12s %s\n", n, title)
				}
				fmt.Fprintln(w, "\n  omega docs <page> · omega docs search <words> · omega docs ask \"<question>\"")
				return nil
			}
			name := strings.TrimSuffix(strings.ToLower(args[0]), ".md")
			text, ok := pages[name]
			if !ok {
				// A word that is not a page is a search.
				return writeSearch(w, args[0])
			}
			return writeMarkdown(w, text)
		},
	}
	c.AddCommand(&cobra.Command{
		Use:   "search <words>...",
		Short: "Search the manual offline",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return writeSearch(cmd.OutOrStdout(), strings.Join(args, " "))
		},
	})
	var provider, model string
	ask := &cobra.Command{
		Use:   "ask <question>...",
		Short: "Ask the manual a question; the configured model answers from it",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := strings.Join(args, " ")
			answer, err := askManual(cmd.Context(), root, q, provider, model)
			w := cmd.OutOrStdout()
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "omega: no model to ask (%v). Here is what the manual says:\n\n", err)
				return writeSearch(w, q)
			}
			return writeMarkdown(w, answer)
		},
	}
	ask.Flags().StringVar(&provider, "provider", "", i18n.T("flag.provider"))
	ask.Flags().StringVar(&model, "model", "", "model to answer with")
	c.AddCommand(ask)
	return c
}

func writeSearch(w io.Writer, q string) error {
	hits := searchManual(q, 5)
	if len(hits) == 0 {
		fmt.Fprintf(w, "Nothing in the manual matches %q. omega --help lists every command.\n", q)
		return nil
	}
	var b strings.Builder
	for _, s := range hits {
		fmt.Fprintf(&b, "## %s › %s\n\n%s\n\n_omega docs %s_\n\n---\n\n", s.title, s.heading, trimBody(s.body, 1500), s.page)
	}
	return writeMarkdown(w, b.String())
}

// writeMarkdown renders Markdown on a terminal and prints it as it is
// anywhere else.
func writeMarkdown(w io.Writer, text string) error {
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) && os.Getenv("NO_COLOR") == "" {
		width, _, err := term.GetSize(int(f.Fd()))
		if err != nil || width <= 0 {
			width = 100
		}
		r, err := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(min(width-2, 110)))
		if err == nil {
			if out, err := r.Render(text); err == nil {
				_, err = io.WriteString(w, out)
				return err
			}
		}
	}
	_, err := io.WriteString(w, text+"\n")
	return err
}

// askManual sends the question, the best sections and the command list to
// the configured model.
func askManual(ctx context.Context, root *cobra.Command, q, provider, model string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	p, plan, err := resolveProvider(cfg, provider, model, "", agent.TaskQuick)
	if err != nil {
		return "", err
	}
	if p == nil {
		return "", errors.New("no provider")
	}
	system := "You are omega's documentation assistant. omega is a command-line tool and desktop app that maintains macOS, Linux and Windows. " +
		"Answer the user's question using only the manual excerpts and the command list below. Give the exact omega commands to type, in code blocks. " +
		"Be short: a few sentences and the commands. If the manual does not cover it, say so and suggest `omega docs search` or the closest command. " +
		"Never invent commands or flags. Reply in " + i18n.Names[i18n.Lang()] + "."
	prompt := "Question: " + q + "\n\n# Manual excerpts\n\n" + manualAnswerContext(q, 6) + "\n\n# Commands\n\n" + commandSummary(root)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	reply, err := p.NewChat(plan.Model, system, nil).Send(ctx, prompt, nil)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(reply.Text) == "" {
		return "", errors.New("the model gave no answer")
	}
	return reply.Text, nil
}
