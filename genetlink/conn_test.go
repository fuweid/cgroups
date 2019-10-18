package genetlink

import (
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestGetFamily(t *testing.T) {
	conn, err := newConnection()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.close()

	target := "nlctrl"

	m := syscall.NetlinkMessage{
		Header: syscall.NlMsghdr{
			Type:  unix.GENL_ID_CTRL,
			Flags: syscall.NLM_F_REQUEST,
		},
		Data: (&GenlMsg{
			Header: GenlMsghdr{
				Command: unix.CTRL_CMD_GETFAMILY,
			},
			Data: (&Attribute{
				Typ:  unix.CTRL_ATTR_FAMILY_NAME,
				Data: append([]byte(target), 0),
			}).Marshal(),
		}).Marshal(),
	}

	msgs, err := conn.doRequest(m)
	if err != nil {
		t.Fatal(err)
	}

	if len(msgs) != 1 {
		t.Fatalf("expected 1 reply message, but got %v", len(msgs))
	}

	name, err := parseFamilyReply(msgs[0], unix.CTRL_ATTR_FAMILY_NAME)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.Trim(name.(string), "\x00")
	if strings.Compare(got, target) != 0 {
		t.Fatalf("expected name is %s, but got %s", target, got)
	}
}

func TestGetCgroupStats(t *testing.T) {
	cli, err := NewTaskstatsClient()
	if err != nil {
		t.Fatalf("failed to new client: %v", err)
	}
	defer cli.Close()

	cpuSubsystem := "/sys/fs/cgroup/cpu"
	_, err = cli.GetCgroupStats(cpuSubsystem)
	if err != nil {
		t.Fatalf("failed to get cgroupstats for %s: %v", cpuSubsystem, err)
	}
}
