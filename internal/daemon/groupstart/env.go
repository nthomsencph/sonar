package groupstart

import (
	"context"
	"errors"

	"github.com/raskrebs/sonar/internal/daemon"
	"github.com/raskrebs/sonar/internal/daemon/rpc"
	"github.com/raskrebs/sonar/internal/groups"
)

// `groups.env` is `groups.start` without the start: it resolves the port of
// every service in a `sonar.yaml` and the environment each one would be given,
// and spawns nothing. A tool that has to reach the services before they are
// up — a test runner, a migration, an editor wiring a worktree — reads it here
// rather than re-deriving the claim keys itself.
func init() {
	daemon.RegisterHandler("groups.env", handleGroupsEnv)
	daemon.RegisterCapability(rpc.CapabilityEnv)
}

func handleGroupsEnv(_ context.Context, req *daemon.Request) (any, error) {
	var p rpc.GroupsEnvParams
	if err := req.Bind(&p); err != nil {
		return nil, err
	}
	cfg, err := resolveConfig(req.Runtime, p.Name, p.ConfigPath)
	if err != nil {
		return nil, err
	}
	group := req.Runtime.Scanner.GroupOf(cfg)
	// The same address book a start uses, so a `port: auto` service that is
	// not running is claimed here exactly as it would be there: the port this
	// call reports is the port a later start binds.
	book := newAddressBook(req.Runtime, cfg, group)

	out := rpc.GroupsEnvResult{
		MutationResult: rpc.MutationResult{OK: true, Affected: []string{}},
		Group:          group,
		ConfigPath:     cfg.Path,
		Services:       []rpc.GroupsEnvService{},
	}
	for _, svc := range cfg.Services {
		ports, err := book.forService(svc)
		if err != nil {
			return nil, envError(err)
		}
		port := ports[svc.Name]
		row := rpc.GroupsEnvService{
			Name:   svc.Name,
			Port:   port,
			Source: book.sources[svc.Name],
			Env:    serviceVars(svc, port, ports),
		}
		if port > 0 {
			row.URL = groups.URL(port)
		}
		out.Services = append(out.Services, row)
	}
	return out, nil
}

// envError is a port failure as the call's error. A claim failure arrives
// from forService with its own code and hint — not_found for an exhausted
// range, claim_conflict for a held port — so a tool reading `sonar env --json`
// can tell either from a daemon bug; anything else is internal.
func envError(err error) error {
	var re *rpc.Error
	if errors.As(err, &re) {
		return re
	}
	return rpc.NewError(rpc.CodeInternal, err.Error(),
		"`sonar claims` lists what is held; `sonar down` releases this project's claims")
}
