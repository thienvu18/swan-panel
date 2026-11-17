package policy

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeDockerIptables struct {
	docker, owned bool
	forward       string
	dockerRules   []string
	ownedRules    []string
	calls         []string
}

func (f *fakeDockerIptables) run(_ context.Context, args ...string) (string, error) {
	call := strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if len(args) < 2 {
		return "", errors.New("invalid command")
	}
	switch args[0] {
	case "-S":
		switch args[1] {
		case "DOCKER-USER":
			if !f.docker {
				return "", errors.New("no such chain")
			}
			return "-N DOCKER-USER\n" + strings.Join(f.dockerRules, "\n") + "\n", nil
		case "FORWARD":
			return "-P FORWARD " + f.forward + "\n", nil
		case dockerForwardChain:
			if !f.owned {
				return "", errors.New("no such chain")
			}
			return "-N " + dockerForwardChain + "\n" + strings.Join(f.ownedRules, "\n") + "\n", nil
		}
	case "-N":
		if args[1] == dockerForwardChain {
			f.owned = true
			return "", nil
		}
	case "-F":
		if args[1] == dockerForwardChain {
			f.ownedRules = nil
			return "", nil
		}
	case "-A":
		if args[1] == dockerForwardChain {
			f.ownedRules = append(f.ownedRules, call)
			return "", nil
		}
	case "-I":
		if len(args) >= 5 && args[1] == "DOCKER-USER" && args[3] == "-j" && args[4] == dockerForwardChain {
			f.dockerRules = append([]string{"-A DOCKER-USER -j " + dockerForwardChain}, f.dockerRules...)
			return "", nil
		}
	}
	return "", errors.New("unexpected iptables command: " + call)
}

func TestDockerHostForwardUsesVIPIPsecAndAuthorizedConnmark(t *testing.T) {
	ctx := context.Background()
	fake := &fakeDockerIptables{docker: true, forward: "DROP"}
	vips := []string{"10.122.0.2/32", "10.250.0.2/32"}
	if err := syncHostDockerForward(ctx, vips, fake.run); err != nil {
		t.Fatal(err)
	}
	needs, err := dockerForwardNeedsSync(ctx, vips, fake.run)
	if err != nil || needs {
		t.Fatalf("installed forward rules drifted: %t %v", needs, err)
	}
	count := 0
	for _, rule := range fake.ownedRules {
		if strings.Contains(rule, "-j ACCEPT") {
			count++
			if !strings.Contains(rule, "-m connmark --mark "+dockerForwardMark) {
				t.Fatalf("ACCEPT without Zero-Trust connmark: %s", rule)
			}
			if strings.Contains(rule, " -s ") && !strings.Contains(rule, "-m policy --dir in --pol ipsec") {
				t.Fatalf("outbound ACCEPT without IPsec origin: %s", rule)
			}
			if strings.Contains(rule, " -d ") && !strings.Contains(rule, "-m conntrack --ctstate ESTABLISHED,RELATED") {
				t.Fatalf("unsolicited response ACCEPT: %s", rule)
			}
		}
	}
	if count != 2*len(vips) {
		t.Fatalf("wrong number of restricted accepts: %d", count)
	}
	for _, call := range fake.calls {
		if strings.Contains(call, "/16") || strings.Contains(call, "-P FORWARD") ||
			strings.Contains(call, "-F DOCKER-USER") || strings.Contains(call, "-F FORWARD") {
			t.Fatalf("unscoped host firewall mutation: %s", call)
		}
	}
	// A same-length tampered chain with an unconditional ACCEPT must be detected.
	fake.ownedRules[0] = strings.Replace(fake.ownedRules[0], "-m connmark --mark "+dockerForwardMark+" ", "", 1)
	if drift, err := dockerForwardNeedsSync(ctx, vips, fake.run); err != nil || !drift {
		t.Fatalf("unmarked ACCEPT was not detected: %t %v", drift, err)
	}
	if err := syncHostDockerForward(ctx, vips, fake.run); err != nil {
		t.Fatal(err)
	}
	if len(fake.dockerRules) != 1 {
		t.Fatalf("duplicated DOCKER-USER jump: %v", fake.dockerRules)
	}
}

func TestDockerHostForwardDriftAndRevocation(t *testing.T) {
	ctx := context.Background()
	fake := &fakeDockerIptables{docker: true, forward: "DROP"}
	if err := syncHostDockerForward(ctx, []string{"10.122.0.2/32"}, fake.run); err != nil {
		t.Fatal(err)
	}
	fake.dockerRules = nil // Docker restart removed our jump.
	if drift, err := dockerForwardNeedsSync(ctx, []string{"10.122.0.2/32"}, fake.run); err != nil || !drift {
		t.Fatalf("missing Docker jump not detected: %t %v", drift, err)
	}
	if err := syncHostDockerForward(ctx, nil, fake.run); err != nil {
		t.Fatal(err)
	}
	if len(fake.ownedRules) != 1 || !strings.Contains(fake.ownedRules[0], "-j RETURN") {
		t.Fatalf("revoked VIP retains forwarding grant: %v", fake.ownedRules)
	}
}

func TestDockerHostForwardNoUnscopedFallback(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		policy string
		fail   bool
	}{{"DROP", true}, {"ACCEPT", false}} {
		fake := &fakeDockerIptables{forward: tc.policy}
		err := syncHostDockerForward(ctx, []string{"10.122.0.2/32"}, fake.run)
		if (err != nil) != tc.fail {
			t.Fatalf("FORWARD %s: err=%v, want error %t", tc.policy, err, tc.fail)
		}
		for _, call := range fake.calls {
			if strings.Contains(call, "-j ACCEPT") {
				t.Fatalf("installed an unscoped fallback: %s", call)
			}
		}
	}
}
