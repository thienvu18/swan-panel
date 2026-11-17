package realtime

import "testing"

func TestSSEPrivilegedPayloadNeverLeaksToOrdinarySubscriber(t *testing.T) {
	private := Event{Type: "user_approved", Payload: map[string]any{"email": "secret@example.org"}, Timestamp: 12}
	if _, allowed := sanitizeEvent(private, false); allowed {
		t.Fatal("ordinary subscriber received an administrator-only event")
	}
	session := Event{Type: "session_terminated", Payload: map[string]any{"ike_name": "secret-IKE"}, Timestamp: 13}
	sanitized, allowed := sanitizeEvent(session, false)
	if !allowed || sanitized.Type != "session_update" || sanitized.Payload != nil {
		t.Fatalf("ordinary subscriber received identifiable session information: %+v", sanitized)
	}
	full, allowed := sanitizeEvent(session, true)
	if !allowed || full.Payload == nil {
		t.Fatal("administrator no longer receives original session information")
	}
}
