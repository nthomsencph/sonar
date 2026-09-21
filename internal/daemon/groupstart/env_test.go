package groupstart

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/raskrebs/sonar/internal/daemon/rpc"
	"github.com/raskrebs/sonar/internal/groups"
)

// TestEnvResolvesPortsWithoutStarting is the contract of `sonar env`: every
// service's port and expanded env come back with nothing started, a `port:
// auto` service gets the same claim on every call, and a start afterwards
// binds exactly the port env reported.
func TestEnvResolvesPortsWithoutStarting(t *testing.T) {
	skipOnWindows(t)
	dir := isolate(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbPort := freePort(t)
	cmd := serviceCmd(t)
	path := writeConfig(t, dir, fmt.Sprintf(`name: envtest
services:
  - name: db
    cmd: %s
    port: %d
  - name: api
    cmd: %s
    port: auto
    depends_on: [db]
    env:
      DB_URL: postgres://localhost:${db.port}/app
      SELF: ${url}
  - name: worker
    cmd: %s
    env:
      API_URL: ${api.url}
`, cmd, dbPort, cmd, cmd))

	watch := &watchList{}
	c := startDaemonWith(t, ctx, watchScan(dir, watch))

	var res rpc.GroupsEnvResult
	if err := c.Call(ctx, "groups.env", rpc.GroupsEnvParams{ConfigPath: &path}, &res); err != nil {
		t.Fatalf("groups.env: %v", err)
	}
	if !res.OK || res.Group == "" || res.ConfigPath != path {
		t.Fatalf("result = %+v", res)
	}
	if len(res.Services) != 3 {
		t.Fatalf("services = %+v, want db, api, worker", res.Services)
	}
	db, api, worker := res.Services[0], res.Services[1], res.Services[2]
	if db.Name != "db" || api.Name != "api" || worker.Name != "worker" {
		t.Fatalf("order = %s, %s, %s; want the file's", db.Name, api.Name, worker.Name)
	}

	if db.Port != dbPort || db.Source != rpc.PortSourceFixed || db.URL != groups.URL(dbPort) {
		t.Errorf("db = %+v, want the fixed port %d", db, dbPort)
	}
	if want := map[string]string{"PORT": fmt.Sprint(dbPort)}; !reflect.DeepEqual(db.Env, want) {
		t.Errorf("db env = %v, want %v", db.Env, want)
	}

	if api.Port < 10000 || api.Port > 32699 {
		t.Errorf("api port = %d, want one from the claim range", api.Port)
	}
	if api.Source != rpc.PortSourceClaimed || api.URL != groups.URL(api.Port) {
		t.Errorf("api = %+v, want a claimed port with its url", api)
	}
	if want := map[string]string{
		"PORT":   fmt.Sprint(api.Port),
		"DB_URL": fmt.Sprintf("postgres://localhost:%d/app", dbPort),
		"SELF":   groups.URL(api.Port),
	}; !reflect.DeepEqual(api.Env, want) {
		t.Errorf("api env = %v, want %v", api.Env, want)
	}

	if worker.Port != 0 || worker.URL != "" || worker.Source != "" {
		t.Errorf("worker = %+v, want no port of its own", worker)
	}
	if want := map[string]string{"API_URL": groups.URL(api.Port)}; !reflect.DeepEqual(worker.Env, want) {
		t.Errorf("worker env = %v, want %v", worker.Env, want)
	}

	// Asking again is the same answer: the claim is idempotent.
	var again rpc.GroupsEnvResult
	if err := c.Call(ctx, "groups.env", rpc.GroupsEnvParams{ConfigPath: &path}, &again); err != nil {
		t.Fatalf("groups.env again: %v", err)
	}
	if again.Services[1].Port != api.Port {
		t.Fatalf("second call gave api %d, first gave %d", again.Services[1].Port, api.Port)
	}

	// And a start binds what env said it would.
	var start rpc.GroupsStartResult
	s, err := c.Stream(ctx, "groups.start", rpc.GroupsStartParams{ConfigPath: &path}, &start)
	if err != nil {
		t.Fatalf("groups.start: %v", err)
	}
	defer s.Close()
	chunks, end := collectWatching(t, s, watch)
	t.Cleanup(func() { killPIDs(chunks) })
	if len(end.Errors) != 0 {
		dumpLogs(t, chunks)
		t.Fatalf("end = %+v, chunks = %+v", end, chunks)
	}
	for _, ch := range chunks {
		if ch.Service == "api" && ch.Port != api.Port {
			t.Errorf("start bound api to %d, env reported %d", ch.Port, api.Port)
		}
	}

	// Running now, api keeps its port and env says why.
	var running rpc.GroupsEnvResult
	if err := c.Call(ctx, "groups.env", rpc.GroupsEnvParams{ConfigPath: &path}, &running); err != nil {
		t.Fatalf("groups.env while running: %v", err)
	}
	if got := running.Services[1]; got.Port != api.Port || got.Source != rpc.PortSourceRunning {
		t.Errorf("api while running = %+v, want port %d from %q", got, api.Port, rpc.PortSourceRunning)
	}
}

func TestEnvNeedsAFile(t *testing.T) {
	skipOnWindows(t)
	dir := isolate(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := startDaemon(t, ctx, dir, nil)

	var res rpc.GroupsEnvResult
	err := c.Call(ctx, "groups.env", rpc.GroupsEnvParams{}, &res)
	if err == nil {
		t.Fatal("groups.env with neither name nor config_path succeeded")
	}
	missing := dir + "/nowhere/" + groups.ConfigName
	if err := c.Call(ctx, "groups.env", rpc.GroupsEnvParams{ConfigPath: &missing}, &res); err == nil {
		t.Fatal("groups.env on a missing file succeeded")
	}
}
