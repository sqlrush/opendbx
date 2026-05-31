// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package db

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeDriver / fakeConn satisfy the interfaces for registry tests without pgx.
type fakeDriver struct{ name string }

func (d fakeDriver) Name() string { return d.name }
func (d fakeDriver) Open(_ context.Context, _ string) (Conn, error) {
	return fakeConn{}, nil
}

type fakeConn struct{}

func (fakeConn) Ping(context.Context) error                  { return nil }
func (fakeConn) HealthCheck(context.Context) (Health, error) { return Health{Reachable: true}, nil }
func (fakeConn) Close() error                                { return nil }

func TestRegisterAndOpen(t *testing.T) {
	Register(fakeDriver{name: "reg-ok"})
	conn, err := Open(context.Background(), "reg-ok", "dsn")
	if err != nil {
		t.Fatalf("Open registered driver: %v", err)
	}
	if conn == nil {
		t.Fatal("Open returned nil conn")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	Register(fakeDriver{name: "reg-dup"})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("duplicate Register did not panic")
		}
	}()
	Register(fakeDriver{name: "reg-dup"}) // must panic
}

func TestOpenUnknownDriver(t *testing.T) {
	_, err := Open(context.Background(), "no-such-driver", "dsn")
	if err == nil {
		t.Fatal("Open unknown driver returned nil error")
	}
	if !errors.Is(err, ErrDriverUnknown) {
		t.Fatalf("want ErrDriverUnknown, got %v", err)
	}
	// The driver name is allowed in the message (not a secret); the Hint must
	// not be empty (规则 7).
	var ec interface{ Hint() string }
	if errors.As(err, &ec) && ec.Hint() == "" {
		t.Error("DB.DRIVER_UNKNOWN has empty Hint")
	}
}

// TestOpenConcurrent exercises the RWMutex: many concurrent Open reads against
// a registered driver while the registry is otherwise quiescent. Run with -race.
func TestOpenConcurrent(t *testing.T) {
	Register(fakeDriver{name: "reg-concurrent"})
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Open(context.Background(), "reg-concurrent", "dsn"); err != nil {
				t.Errorf("concurrent Open: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestLookup(t *testing.T) {
	Register(fakeDriver{name: "lookup-ok"})
	d, ok := Lookup("lookup-ok")
	if !ok || d.Name() != "lookup-ok" {
		t.Errorf("Lookup registered = (%v, %v)", d, ok)
	}
	if _, ok := Lookup("lookup-missing"); ok {
		t.Error("Lookup missing reported found")
	}
}

func TestRegisteredNames(t *testing.T) {
	Register(fakeDriver{name: "reg-named"})
	found := false
	for _, n := range registeredNames() {
		if n == "reg-named" {
			found = true
		}
	}
	if !found {
		t.Error("registeredNames did not include reg-named")
	}
}
