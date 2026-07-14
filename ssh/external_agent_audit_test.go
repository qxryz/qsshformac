package ssh

import (
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestParseExternalAgentAuditDoesNotExposePublicKey(t *testing.T) {
	const output = "__QSSH_EXTERNAL_SCOPE__\tall\n" +
		"__QSSH_EXTERNAL_KEY__\t/root/.ssh/authorized_keys\tssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKgV3NIV8l5O2KTJ2WcH2O9qOpmG4Bz21PH8b4m9S8TQ codex-example-deploy\n" +
		"__QSSH_EXTERNAL_WHO__\nzhouzhou-deploy pts/1 2026-07-14 09:30 (192.0.2.10)\n" +
		"root pts/2 2026-07-14 09:31 (192.0.2.11)\n" +
		"__QSSH_EXTERNAL_AUTH__\nAccepted publickey for zhouzhou-deploy from 192.0.2.10 port 44221 ssh2: ED25519 SHA256:example-agent-fingerprint\n" +
		"Accepted password for root from 192.0.2.11 port 44222 ssh2\n"

	audit := parseExternalAgentAudit(output)
	if !audit.CanInspectAll || len(audit.Keys) != 1 || len(audit.Sessions) != 4 {
		t.Fatalf("unexpected audit: %#v", audit)
	}
	if audit.Keys[0].Username != "root" || audit.Keys[0].Comment != "codex-example-deploy" {
		t.Fatalf("unexpected key metadata: %#v", audit.Keys[0])
	}
	if audit.Keys[0].Fingerprint == "" || audit.Keys[0].Algorithm != "ssh-ed25519" {
		t.Fatalf("missing safe key metadata: %#v", audit.Keys[0])
	}
	if !audit.Sessions[0].Active || audit.Sessions[0].AuthMethod != "publickey" || audit.Sessions[0].Fingerprint != "SHA256:example-agent-fingerprint" {
		t.Fatalf("agent session was not attributed by fingerprint: %#v", audit.Sessions[0])
	}
	if audit.Sessions[1].AuthMethod != "password" || audit.Sessions[1].Fingerprint != "" {
		t.Fatalf("password root session attribution failed: %#v", audit.Sessions[1])
	}
}

func TestUsernameFromAuthorizedKeysPath(t *testing.T) {
	cases := map[string]string{
		"/root/.ssh/authorized_keys":                 "root",
		"/home/zhouzhou-deploy/.ssh/authorized_keys": "zhouzhou-deploy",
		"/custom/home/.ssh/authorized_keys":          "current-user",
	}
	for path, want := range cases {
		if got := usernameFromAuthorizedKeysPath(path); got != want {
			t.Fatalf("%s: got %q want %q", path, got, want)
		}
	}
}

func TestParseExternalAgentAuditLeavesNottySessionUnattributed(t *testing.T) {
	const output = "__QSSH_EXTERNAL_SCOPE__\tall\n" +
		"__QSSH_EXTERNAL_WHO__\n" +
		"__QSSH_EXTERNAL_PROCESS__\n 4242 3 sshd: zhouzhou-deploy@notty\n" +
		"__QSSH_EXTERNAL_AUTH__\n2026-07-14T17:30:00+08:00 host sshd[4242]: Accepted publickey for zhouzhou-deploy from 192.0.2.10 port 44221 ssh2: ED25519 SHA256:example-agent-fingerprint\n"

	audit := parseExternalAgentAudit(output)
	if len(audit.Sessions) != 2 {
		t.Fatalf("expected active and recent session, got %#v", audit.Sessions)
	}
	active := audit.Sessions[0]
	if !active.Active || active.Terminal != "notty" || active.PID != 4242 || active.Fingerprint != "" || active.AuthMethod != "" {
		t.Fatalf("notty session must remain unconfirmed without an exact source match: %#v", active)
	}
}

func TestParseExternalAgentAuditExcludesOnlyItsOwnNottyProcess(t *testing.T) {
	const output = "__QSSH_EXTERNAL_SCOPE__\tall\n" +
		"__QSSH_EXTERNAL_WHO__\n" +
		"__QSSH_EXTERNAL_SELF_PROCESS__\t4242\n" +
		"__QSSH_EXTERNAL_PROCESS__\n 4242 3 sshd: root@notty\n 4243 3 sshd: root@notty\n" +
		"__QSSH_EXTERNAL_AUTH__\n"

	audit := parseExternalAgentAudit(output)
	if len(audit.Sessions) != 1 {
		t.Fatalf("expected only the unrelated notty session, got %#v", audit.Sessions)
	}
	if audit.Sessions[0].PID != 4243 || audit.Sessions[0].Username != "root" || audit.Sessions[0].Terminal != "notty" {
		t.Fatalf("unexpected remaining session: %#v", audit.Sessions[0])
	}
}

func TestRemoveAuthorizedKeyByFingerprintPreservesOtherLines(t *testing.T) {
	const target = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKgV3NIV8l5O2KTJ2WcH2O9qOpmG4Bz21PH8b4m9S8TQ codex-example-deploy\n"
	const content = "# keep this comment\n" + target + "\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDKk1G2x7EiQhzSxJ6vTrzN6UsBzA2T5S0C3xKByZlT3 user-key\n"
	pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(target))
	if err != nil {
		t.Fatal(err)
	}
	got, removed := removeAuthorizedKeyByFingerprint(content, gossh.FingerprintSHA256(pub))
	if !removed || got != "# keep this comment\n\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDKk1G2x7EiQhzSxJ6vTrzN6UsBzA2T5S0C3xKByZlT3 user-key\n" {
		t.Fatalf("unexpected revoke result: removed=%v content=%q", removed, got)
	}
}

func TestCanRevokeExternalAgentKeyForUser(t *testing.T) {
	if !canRevokeExternalAgentKeyForUser("root", "root") {
		t.Fatal("current account must be able to revoke its own key")
	}
	if !canRevokeExternalAgentKeyForUser("root", "deploy-agent") {
		t.Fatal("root must be able to administer an agent service account")
	}
	if canRevokeExternalAgentKeyForUser("deploy-agent", "root") {
		t.Fatal("non-root account must not revoke another user's key")
	}
}

func TestExternalAgentCommandAsUserDropsRootForServiceUser(t *testing.T) {
	command := externalAgentCommandAsUser("root", "deploy-agent", "cat \"$HOME/.ssh/authorized_keys\"")
	if !strings.Contains(command, "runuser -u \"$target\"") || !strings.Contains(command, "HOME=\"$home\"") {
		t.Fatalf("root command must drop to the target account: %s", command)
	}
	if got := externalAgentCommandAsUser("deploy-agent", "deploy-agent", "id -un"); got != "id -un" {
		t.Fatalf("current user command should not wrap itself: %q", got)
	}
}
