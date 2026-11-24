package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/netdiag"
	"github.com/lcajigasm/omega/internal/ui"
)

// Seams of net kill, replaced by the tests.
var (
	findPorts   = netdiag.FindPorts
	stopProcess = netdiag.StopProcess
)

func netCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "net",
		Short: i18n.T("cmd.net"),
		Long:  i18n.T("cmd.net_long"),
		Example: "  omega net ping\n" +
			"  omega net ports\n" +
			"  omega net ports 3000\n" +
			"  omega net kill 3000\n" +
			"  omega net audit --json",
	}

	var asJSON, udp, yes bool
	ping := &cobra.Command{
		Use:   "ping",
		Short: i18n.T("cmd.net_ping"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			results := netdiag.PingRegistries(cmd.Context())
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), results)
			}
			writePing(cmd.OutOrStdout(), results)
			return nil
		},
	}
	ping.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))

	ports := &cobra.Command{
		Use:   "ports [port]",
		Short: i18n.T("cmd.net_ports"),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPort := 0
			if len(args) == 1 {
				// A word that is not a port used to list every port.
				port, err := parsePortArg(args[0])
				if err != nil {
					return err
				}
				targetPort = port
			}
			proto := "tcp"
			if udp {
				proto = "udp"
			}
			list, err := findPorts(cmd.Context(), targetPort, proto)
			if err != nil {
				return err
			}
			if asJSON {
				if list == nil {
					list = []netdiag.PortInfo{}
				}
				return writeJSON(cmd.OutOrStdout(), list)
			}
			writePorts(cmd.OutOrStdout(), list, targetPort, udp)
			return nil
		},
	}
	ports.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))
	ports.Flags().BoolVar(&udp, "udp", false, i18n.T("flag.net_udp"))

	kill := &cobra.Command{
		Use:     "kill <port>",
		Short:   i18n.T("cmd.net_kill"),
		Long:    i18n.T("cmd.net_kill_long"),
		Example: "  omega net kill 3000\n  omega net kill 5173 --yes",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			port, err := parsePortArg(args[0])
			if err != nil {
				return err
			}
			return killPort(cmd.Context(), cmd.OutOrStdout(), port, yes, stdinIsTerminal(), askYesNo)
		},
	}
	kill.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.tools_yes"))

	audit := &cobra.Command{
		Use:   "audit",
		Short: i18n.T("cmd.net_audit"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := netdiag.AuditHostsAndProxies()
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), a)
			}
			writeNetAudit(cmd.OutOrStdout(), a)
			return nil
		},
	}
	audit.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))

	c.AddCommand(
		&cobra.Command{
			Use:   "flush",
			Short: i18n.T("cmd.net_flush"),
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				msg, err := netdiag.FlushDNS(cmd.Context())
				if err != nil {
					return err
				}
				r := ui.NewReport(cmd.OutOrStdout())
				r.Check(ui.MarkOK, msg, 0, "")
				return nil
			},
		},
		ping, ports, kill, audit,
	)
	return c
}

func parsePortArg(s string) (int, error) {
	port, err := strconv.Atoi(s)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s", i18n.T("net.bad_port", s))
	}
	return port, nil
}

func writePing(w io.Writer, results []netdiag.RegistryPing) {
	r := ui.NewReport(w)
	r.Title("omega net ping", i18n.T("net.ping_detail", len(results)))
	r.Blank()
	var rows [][]string
	var marks []ui.Mark
	for _, p := range results {
		mark, status := ui.MarkOK, i18n.T("net.reachable", p.Status)
		switch {
		case !p.Reachable:
			mark, status = ui.MarkErr, r.Err(i18n.T("net.unreachable", r.Clip(p.Error, 60)))
		case p.Status < 400:
			status = i18n.T("net.ok_http", p.Status)
		}
		latency := r.Muted("—")
		if p.Reachable {
			latency = p.Latency.Round(time.Millisecond).String()
		}
		rows = append(rows, []string{p.Name, r.Muted(p.URL), latency, status})
		marks = append(marks, mark)
	}
	r.Table([]ui.Column{{Title: i18n.T("net.col_registry")}, {Title: i18n.T("net.col_endpoint")},
		{Title: i18n.T("net.col_latency"), Right: true}, {Title: i18n.T("net.col_status")}}, rows, marks)
	r.Blank()
	r.Hint(i18n.T("net.ping_hint"))
}

func writePorts(w io.Writer, ports []netdiag.PortInfo, target int, udp bool) {
	if len(ports) == 0 {
		switch {
		case target > 0:
			fmt.Fprintln(w, i18n.T("net.port_free", target))
		case udp:
			fmt.Fprintln(w, i18n.T("net.no_udp"))
		default:
			fmt.Fprintln(w, i18n.T("net.no_ports"))
		}
		return
	}
	sort.SliceStable(ports, func(i, j int) bool { return ports[i].Port < ports[j].Port })
	r := ui.NewReport(w)
	r.Title("omega net ports", i18n.T("net.ports_detail", len(ports)))
	r.Blank()
	var rows [][]string
	for _, p := range ports {
		rows = append(rows, []string{strconv.Itoa(p.Port), p.Proto, strconv.Itoa(p.PID), p.Process, r.Muted(p.User)})
	}
	r.Table([]ui.Column{{Title: i18n.T("net.col_port"), Right: true}, {Title: i18n.T("net.col_proto")},
		{Title: "PID", Right: true}, {Title: i18n.T("net.col_process")}, {Title: i18n.T("net.col_user")}}, rows, nil)
	r.Blank()
	r.Hint(i18n.T("net.ports_hint"))
}

// killPort ends what listens on a TCP port: it says which process that is,
// asks (unless yes), asks the process to exit and forces it only when it
// does not within netdiag.StopGrace.
func killPort(ctx context.Context, w io.Writer, port int, yes, tty bool, confirm func(string) bool) error {
	ports, err := findPorts(ctx, port, "tcp")
	if err != nil {
		return err
	}
	// One line per process: a process on IPv4 and IPv6 is listed once.
	seen := map[int]bool{}
	var targets []netdiag.PortInfo
	for _, p := range ports {
		if p.PID > 1 && !seen[p.PID] {
			seen[p.PID] = true
			targets = append(targets, p)
		}
	}
	if len(targets) == 0 {
		return fmt.Errorf("%s", i18n.T("net.port_free", port))
	}
	r := ui.NewReport(w)
	for _, p := range targets {
		r.Check(ui.MarkWarn, i18n.T("net.kill_what", p.PID, p.Process, p.Port), 0, "")
	}
	if !yes {
		if !tty {
			return errors.New(i18n.T("net.kill_need_yes"))
		}
		if !confirm(i18n.N("net.kill_confirm", len(targets))) {
			fmt.Fprintln(w, i18n.T("net.kill_cancelled"))
			return nil
		}
	}
	failed := 0
	for _, p := range targets {
		forced, err := stopProcess(ctx, p.PID, netdiag.StopGrace)
		switch {
		case err != nil:
			failed++
			r.Check(ui.MarkErr, i18n.T("net.kill_failed", p.PID, p.Process), 0, err.Error())
		case forced:
			r.Check(ui.MarkOK, i18n.T("net.kill_forced", p.PID, p.Process), 0, "")
		default:
			r.Check(ui.MarkOK, i18n.T("net.kill_done", p.PID, p.Process), 0, "")
		}
	}
	if failed > 0 {
		return fmt.Errorf("%s", i18n.T("net.kill_some_failed", failed, len(targets)))
	}
	return nil
}

func writeNetAudit(w io.Writer, a *netdiag.NetworkAudit) {
	r := ui.NewReport(w)
	r.Title("omega net audit", "")
	r.Section(i18n.T("net.hosts_section"), len(a.CustomHosts))
	r.Line(r.Muted(a.HostsFile))
	if len(a.CustomHosts) == 0 {
		r.Check(ui.MarkOK, i18n.T("net.hosts_stock"), 0, "")
	} else {
		var rows [][]string
		for _, h := range a.CustomHosts {
			rows = append(rows, []string{h.IP, h.Hostname})
		}
		r.Table([]ui.Column{{Title: "IP"}, {Title: i18n.T("net.col_host")}}, rows, nil)
	}
	r.Section(i18n.T("net.proxy_section"), len(a.ProxyEnv))
	if len(a.ProxyEnv) == 0 {
		r.Check(ui.MarkOK, i18n.T("net.no_proxy"), 0, "")
	} else {
		keys := make([]string, 0, len(a.ProxyEnv))
		for k := range a.ProxyEnv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		width := 0
		for _, k := range keys {
			width = max(width, len(k))
		}
		for _, k := range keys {
			r.Check(ui.MarkInfo, k, width, a.ProxyEnv[k])
		}
	}
	if len(a.Warnings) > 0 {
		r.Section(i18n.T("net.warnings_section"), len(a.Warnings))
		for _, wn := range a.Warnings {
			r.Check(ui.MarkWarn, wn, 0, "")
		}
	}
	r.Blank()
}
