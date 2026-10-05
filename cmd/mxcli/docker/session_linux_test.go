// SPDX-License-Identifier: Apache-2.0

//go:build linux

package docker

import (
	"os/exec"
	"slices"
	"syscall"
	"testing"
	"time"
)

func TestParseProcStat_CommWithSpacesAndParens(t *testing.T) {
	line := "4242 (my (odd) proc) S 1 4242 4200 0 -1 4194560 100 0 0 0 1 2 0 0 20 0 3 0 987654 1000 10 18446744073709551615"
	st, ok := parseProcStat(line)
	if !ok || st.state != "S" || st.session != 4200 || st.startTicks != 987654 {
		t.Fatalf("got %+v ok=%v", st, ok)
	}
	if _, ok := parseProcStat("garbage"); ok {
		t.Error("garbage parsed")
	}
}

// A session leader's grandchild stays in the session after the leader is
// gone — which is exactly the orphan `run stop` has to find.
func TestSessionMembers_FindsOrphanedGrandchild(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30 & echo started; wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, _ := cmd.StdoutPipe()
	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sh: %v", err)
	}
	buf := make([]byte, 16)
	_, _ = out.Read(buf)
	leader := cmd.Process.Pid
	members := SessionMembers(leader, start)
	if !slices.Contains(members, leader) || len(members) < 2 {
		t.Fatalf("members of session %d = %v, want the leader and its sleep", leader, members)
	}
	_ = syscall.Kill(leader, syscall.SIGKILL)
	_ = cmd.Wait()
	if PidAlive(leader) {
		t.Fatal("leader reaped but still alive")
	}
	left := SessionMembers(leader, start)
	if len(left) != 1 {
		t.Fatalf("after the leader died, members = %v; want the orphaned sleep", left)
	}
	_ = syscall.Kill(left[0], syscall.SIGKILL)

	// The start-time guard: a session observed with a notBefore after its
	// processes started is treated as someone else's (pid reuse).
	if got := SessionMembers(leader, time.Now().Add(time.Hour)); len(got) != 0 {
		t.Errorf("start-time guard let %v through", got)
	}
}
