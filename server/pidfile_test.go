package server

import (
	"os"
	"os/exec"
	"testing"
)

func TestSameExecutablePathTreatsAReplacedBinaryAsTheSamePath(t *testing.T) {
	cases := []struct {
		ownExecutable, otherExecutable string
		want                           bool
	}{
		{"/opt/bin/inber-server", "/opt/bin/inber-server", true},
		{"/opt/bin/inber-server", "/opt/bin/inber-server (deleted)", true},
		{"/opt/bin/inber-server (deleted)", "/opt/bin/inber-server", true},
		{"/opt/bin/inber-server", "/usr/bin/bash", false},
		{"/opt/bin/inber-server", "/opt/bin/inber-server-old", false},
	}
	for _, c := range cases {
		if got := sameExecutablePath(c.ownExecutable, c.otherExecutable); got != c.want {
			t.Errorf("sameExecutablePath(%q, %q) = %v, want %v", c.ownExecutable, c.otherExecutable, got, c.want)
		}
	}
}

func TestRunsThisExecutableIsTrueForThisProcess(t *testing.T) {
	if !runsThisExecutable(os.Getpid()) {
		t.Fatal("this process does not run its own executable")
	}
}

// The case the old command-line match got wrong: a shell whose arguments
// mention "inber" and "serve" is not an inber-server and must not be killed.
func TestRunsThisExecutableIsFalseForAShellThatMentionsInberServe(t *testing.T) {
	shell := exec.Command("sh", "-c", "sleep 30 # repos/inber/server serve")
	if err := shell.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = shell.Process.Kill()
		_ = shell.Wait()
	}()
	if runsThisExecutable(shell.Process.Pid) {
		t.Fatalf("pid %d (sh) was taken for this executable", shell.Process.Pid)
	}
}

func TestRunsThisExecutableIsFalseForAPidWithNoProcess(t *testing.T) {
	// Above the kernel's pid_max ceiling (2^22), so no process can hold it.
	if runsThisExecutable(1 << 23) {
		t.Fatal("a pid with no process was taken for this executable")
	}
}
