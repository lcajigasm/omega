package cli

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/netdiag"
)

func netCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "net",
		Short: "Network, DNS and connectivity maintenance",
		Long:  "Tools for flushing DNS caches, testing registry latency, finding port conflicts, and auditing hosts/proxies.",
	}

	c.AddCommand(
		&cobra.Command{
			Use:   "flush",
			Short: "Flush the system DNS resolver cache",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				msg, err := netdiag.FlushDNS(context.Background())
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "✓ "+msg)
				return nil
			},
		},
		&cobra.Command{
			Use:   "ping",
			Short: "Test reachability and latency to package registries and mirrors",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				results := netdiag.PingRegistries(context.Background())
				fmt.Fprintf(cmd.OutOrStdout(), "\n%-18s %-32s %-12s %s\n", "Registry", "Endpoint", "Latency", "Status")
				fmt.Fprintln(cmd.OutOrStdout(), stringsRepeat("─", 75))
				for _, r := range results {
					status := fmt.Sprintf("%d OK", r.Status)
					if !r.Reachable {
						status = "FAIL (" + r.Error + ")"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%-18s %-32s %-12s %s\n", r.Name, cut(r.URL, 32), r.Latency.Round(1e6), status)
				}
				fmt.Fprintln(cmd.OutOrStdout())
				return nil
			},
		},
		&cobra.Command{
			Use:   "ports [port]",
			Short: "Find listening TCP ports or check a specific port",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				targetPort := 0
				if len(args) == 1 {
					// A word that is not a port used to list every port.
					port, err := strconv.Atoi(args[0])
					if err != nil || port < 1 || port > 65535 {
						return fmt.Errorf("invalid port number: %s", args[0])
					}
					targetPort = port
				}
				ports, err := netdiag.FindPorts(context.Background(), targetPort)
				if err != nil {
					return err
				}
				if len(ports) == 0 {
					if targetPort > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "No process found listening on port %d.\n", targetPort)
					} else {
						fmt.Fprintln(cmd.OutOrStdout(), "No active listening TCP ports found.")
					}
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n%-8s %-8s %-20s %s\n", "Port", "PID", "Process", "Protocol")
				fmt.Fprintln(cmd.OutOrStdout(), stringsRepeat("─", 50))
				for _, p := range ports {
					fmt.Fprintf(cmd.OutOrStdout(), "%-8d %-8d %-20s %s\n", p.Port, p.PID, cut(p.Process, 20), p.Proto)
				}
				fmt.Fprintln(cmd.OutOrStdout())
				return nil
			},
		},
		&cobra.Command{
			Use:   "kill <port>",
			Short: "Terminate the process occupying a specific port",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				port, err := strconv.Atoi(args[0])
				if err != nil || port < 1 || port > 65535 {
					return fmt.Errorf("invalid port number: %s", args[0])
				}
				if err := netdiag.KillPortProcess(context.Background(), port); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "✓ Process listening on port %d terminated.\n", port)
				return nil
			},
		},
		&cobra.Command{
			Use:   "audit",
			Short: "Audit hosts file and proxy environment variables",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				audit, err := netdiag.AuditHostsAndProxies()
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nHosts file: %s\n", audit.HostsFile)
				if len(audit.CustomHosts) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "  ✓ No non-standard hosts entries found.")
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "  Custom entries (%d):\n", len(audit.CustomHosts))
					for _, h := range audit.CustomHosts {
						fmt.Fprintf(cmd.OutOrStdout(), "    %-18s %s\n", h.IP, h.Hostname)
					}
				}

				fmt.Fprintf(cmd.OutOrStdout(), "\nProxy environment:\n")
				if len(audit.ProxyEnv) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "  ✓ No active proxy environment variables.")
				} else {
					for k, v := range audit.ProxyEnv {
						fmt.Fprintf(cmd.OutOrStdout(), "    %s = %s\n", k, v)
					}
				}

				if len(audit.Warnings) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "\nWarnings:\n")
					for _, w := range audit.Warnings {
						fmt.Fprintf(cmd.OutOrStdout(), "  ⚠ %s\n", w)
					}
				}
				fmt.Fprintln(cmd.OutOrStdout())
				return nil
			},
		},
	)

	return c
}

func stringsRepeat(s string, count int) string {
	res := ""
	for i := 0; i < count; i++ {
		res += s
	}
	return res
}
