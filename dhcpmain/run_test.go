package dhcpmain

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeApp is a dhcp app whose servers fail when the test tells them to.
type fakeApp struct {
	failed chan error
}

func newFakeApp() *fakeApp {
	return &fakeApp{failed: make(chan error, 1)}
}

func (a *fakeApp) Failed() <-chan error {
	return a.failed
}

// fakeCaddy records the configurations a reloader stops and loads, and runs
// a new fake app for each configuration it loads.
type fakeCaddy struct {
	loadErrs []error // the errors of the next loads, if any

	stops int
	loads [][]byte
	app   *fakeApp
}

// use makes r stop and load configurations with c, which runs app.
func (c *fakeCaddy) use(r *reloader, app *fakeApp) {
	c.app = app

	r.stop = func() error {
		c.stops++
		c.app = nil
		return nil
	}

	r.load = func(cfgJSON []byte, _ bool) error {
		c.loads = append(c.loads, cfgJSON)

		var err error
		if len(c.loadErrs) > 0 {
			err, c.loadErrs = c.loadErrs[0], c.loadErrs[1:]
		}

		if err == nil {
			c.app = newFakeApp()
		}

		return err
	}

	r.app = func() (failer, error) {
		if c.app == nil {
			return nil, errors.New("no app runs")
		}

		return c.app, nil
	}
}

// writeDhcpfile writes a Dhcpfile with content and returns its name.
func writeDhcpfile(t *testing.T, content string) string {
	t.Helper()

	file := filepath.Join(t.TempDir(), "Dhcpfile")
	require.NoError(t, os.WriteFile(file, []byte(content), 0o600))

	return file
}

func TestReload(t *testing.T) {
	running := []byte(`{"running":true}`)
	file := writeDhcpfile(t, "127.0.0.1/8 {\n}\n")

	expected, err := adapt(file, []byte("127.0.0.1/8 {\n}\n"))
	require.NoError(t, err)

	t.Run("loaded", func(t *testing.T) {
		r := newReloader(running)
		c := new(fakeCaddy)
		c.use(r, newFakeApp())

		require.NoError(t, r.reload(file, "dhcpfile"))

		// the running configuration was stopped before the new one loaded
		assert.Equal(t, 1, c.stops)
		assert.Equal(t, [][]byte{expected}, c.loads)
		assert.Equal(t, expected, r.running)
		assert.Len(t, r.reloaded, 1)
		assert.Empty(t, r.failed)
	})

	t.Run("nothing waits", func(t *testing.T) {
		r := newReloader(running)
		c := new(fakeCaddy)
		c.use(r, newFakeApp())

		// reload runs on the goroutine that handles signals, so it must not
		// block when nothing receives its notifications
		done := make(chan struct{})
		go func() {
			defer close(done)

			for range 3 {
				assert.NoError(t, r.reload(file, "dhcpfile"))
			}
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("reload blocked")
		}
	})

	t.Run("invalid Dhcpfile", func(t *testing.T) {
		for name, file := range map[string]string{
			"syntax error": writeDhcpfile(t, "127.0.0.1/8 {\n"),
			"missing":      filepath.Join(t.TempDir(), "Dhcpfile"),
		} {
			r := newReloader(running)
			c := new(fakeCaddy)
			c.use(r, newFakeApp())

			// the running configuration keeps running
			assert.Error(t, r.reload(file, "dhcpfile"), name)
			assert.Equal(t, 0, c.stops, name)
			assert.Empty(t, c.loads, name)
			assert.Equal(t, running, r.running, name)
			assert.Empty(t, r.reloaded, name)
		}
	})

	t.Run("new configuration fails", func(t *testing.T) {
		r := newReloader(running)
		c := &fakeCaddy{loadErrs: []error{errors.New("port in use")}}
		c.use(r, newFakeApp())

		err := r.reload(file, "dhcpfile")
		assert.ErrorContains(t, err, "port in use")
		assert.ErrorContains(t, err, "previous configuration runs again")

		// the running configuration was loaded again
		assert.Equal(t, [][]byte{expected, running}, c.loads)
		assert.Equal(t, running, r.running)
		assert.Len(t, r.reloaded, 1)
		assert.Empty(t, r.failed)
	})

	t.Run("previous configuration fails too", func(t *testing.T) {
		r := newReloader(running)
		c := &fakeCaddy{loadErrs: []error{errors.New("port in use"), errors.New("file locked")}}
		c.use(r, newFakeApp())

		done := make(chan error, 1)
		go func() { done <- r.wait() }()

		err := r.reload(file, "dhcpfile")
		assert.ErrorContains(t, err, "port in use")
		assert.ErrorContains(t, err, "file locked")

		// no configuration runs, so Run exits
		select {
		case waitErr := <-done:
			assert.Equal(t, err, waitErr)
		case <-time.After(5 * time.Second):
			t.Fatal("wait didn't return")
		}
	})
}

// TestWaitFollowsReloads checks that wait returns the error of a server of
// the configuration that runs after a reload, not only of the first one.
func TestWaitFollowsReloads(t *testing.T) {
	file := writeDhcpfile(t, "127.0.0.1/8 {\n}\n")

	r := newReloader([]byte(`{"running":true}`))
	c := new(fakeCaddy)
	c.use(r, newFakeApp())

	done := make(chan error, 1)
	go func() { done <- r.wait() }()

	for range 2 {
		r.mu.Lock()
		previous := c.app
		r.mu.Unlock()

		require.NoError(t, r.reload(file, "dhcpfile"))

		r.mu.Lock()
		current := c.app
		r.mu.Unlock()
		require.NotSame(t, previous, current)
	}

	r.mu.Lock()
	c.app.failed <- errors.New("interface went away")
	r.mu.Unlock()

	select {
	case err := <-done:
		assert.EqualError(t, err, "interface went away")
	case <-time.After(5 * time.Second):
		t.Fatal("wait didn't return")
	}
}
