package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/raskrebs/sonar/internal/daemon/client"
	"github.com/raskrebs/sonar/internal/daemon/rpc"
	"github.com/raskrebs/sonar/internal/display"
	"github.com/spf13/cobra"
)

var envJSON bool

var envCmd = &cobra.Command{
	Use:   "env [service]",
	Short: "Show the ports and environment sonar start would give the services",
	Long: `Show the ports and environment sonar start would give the services.

No service is started. Every service in the sonar.yaml at or above the current
directory is listed with its port, its URL and its env: with every ${…}
reference expanded — the same values a start sets. A port: auto service that
is not running is given its claim here, the same claim a start makes, so what
this prints is what a later start binds; a service that is already running
keeps the port it is on.

Named, one service is printed as export lines, for a shell that has to reach
the same services the file describes:

  eval "$(sonar env frontend)" && npm test

With --json, the whole answer is printed as one object; with a service name
as well, only that service's entry is.

Examples:
  sonar env                   # every service: port, url and expanded env
  sonar env frontend          # one service, as export lines
  sonar env --json            # for tools`,
	Args: cobra.MaximumNArgs(1),
	RunE: envRun,
}

func init() {
	envCmd.Flags().BoolVar(&envJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(envCmd)
}

func envRun(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolving the working directory: %w", err)
	}
	cfg, err := nearestConfig(wd)
	if err != nil {
		return err
	}
	// A name is checked against the file before the daemon is asked, the way
	// `sonar start <service>` does: a typo should not claim a port for every
	// `port: auto` service in the file on its way to an error.
	name := ""
	if len(args) == 1 {
		name = strings.TrimSpace(args[0])
		if _, ok := cfg.ServiceNamed(name); !ok {
			return unknownService(cfg, name)
		}
	}

	c, err := connectForWrite(cmd.Context())
	if err != nil {
		return err
	}
	defer c.Close()

	if err := requireConfigSupport(c, cfg); err != nil {
		return err
	}
	if err := requireEnvSupport(c); err != nil {
		return err
	}

	var res rpc.GroupsEnvResult
	if err := c.Call(cmd.Context(), "groups.env", rpc.GroupsEnvParams{ConfigPath: &cfg.Path}, &res); err != nil {
		return daemonError(err)
	}

	if name != "" {
		row, ok := serviceRow(res, name)
		if !ok {
			return fmt.Errorf("the daemon's %s does not declare %s; it may have changed since it was read",
				shortPath(res.ConfigPath), name)
		}
		if envJSON {
			return writeJSON(row)
		}
		printEnvExports(row)
		return nil
	}
	if envJSON {
		return writeJSON(res)
	}
	return printEnvTable(res)
}

// serviceRow picks one service out of the daemon's answer by name.
func serviceRow(res rpc.GroupsEnvResult, name string) (rpc.GroupsEnvService, bool) {
	for _, svc := range res.Services {
		if svc.Name == name {
			return svc, true
		}
	}
	return rpc.GroupsEnvService{}, false
}

// requireEnvSupport refuses a daemon that predates `groups.env`, the way
// requireConfigSupport refuses one that predates `port: auto`: the daemon's
// own answer would be the dispatcher's unknown-method error, which says
// nothing about restarting.
func requireEnvSupport(c *client.Client) error {
	hello := c.Hello()
	for _, capability := range hello.Capabilities {
		if capability == rpc.CapabilityEnv {
			return nil
		}
	}
	version := hello.DaemonVersion
	if version == "" {
		version = "an older version"
	}
	return fmt.Errorf("the running daemon (%s) does not know `sonar env`\nhint: restart it with `sonar daemon restart`", version)
}

// printEnvExports prints one service's variables as export lines, so
// `eval "$(sonar env <service>)"` gives a shell what the service would see.
func printEnvExports(svc rpc.GroupsEnvService) {
	for _, line := range envExports(svc.Env) {
		fmt.Println(line)
	}
}

// envExports renders variables as `export NAME=value` lines, sorted by name
// so the output is stable, each value quoted for the shell where it needs it.
func envExports(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, "export "+k+"="+shellQuote(env[k]))
	}
	return out
}

// printEnvTable is the human view: one row per service with its port, its
// URL and its own variables, PORT left out because the port column says it.
func printEnvTable(res rpc.GroupsEnvResult) error {
	if len(res.Services) == 0 {
		fmt.Printf("%s declares no services\n", shortPath(res.ConfigPath))
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
		display.Dim("SERVICE"), display.Dim("PORT"), display.Dim("URL"), display.Dim("ENV"))
	for _, svc := range res.Services {
		port, url := "-", "-"
		if svc.Port > 0 {
			port, url = fmt.Sprint(svc.Port), svc.URL
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", svc.Name, port, url, envSummary(svc.Env))
	}
	return w.Flush()
}

// envSummary joins a service's variables as NAME=value, sorted, without the
// PORT the table already shows.
func envSummary(env map[string]string) string {
	keys := make([]string, 0, len(env))
	for k := range env {
		if k != "PORT" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+env[k])
	}
	return strings.Join(parts, " ")
}
