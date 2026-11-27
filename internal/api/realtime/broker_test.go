package realtime

import "testing"

func TestSSEPrivilegedPayloadNeverLeaksToOrdinarySubscriber(t *testing.T) {
	private := Event{Type: "user_approved", Payload: map[string]any{"email": "secret@example.org"}, Timestamp: 12}
	if _, allowed := sanitizeEvent(private, false); allowed {
		t.Fatal("ordinary subscriber received an administrator-only event")
	}
	auditEv := Event{Type: "audit_event", Payload: map[string]any{"action": "device.revoked"}, Timestamp: 12}
	if _, allowed := sanitizeEvent(auditEv, false); allowed {
		t.Fatal("ordinary subscriber received an administrator-only audit_event")
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

func TestBrokerCloseAllClosesSubscribersCleanly(t *testing.T) {
	b := NewBroker()
	sub := make(chan Event, 2)
	b.newClients <- sub
	b.Broadcast("session_update", nil)
	b.CloseAll()
	// Calling CloseAll twice or broadcasting afterward should remain safe.
	b.CloseAll()
	var nilBroker *Broker
	nilBroker.CloseAll()
	for {
		if _, ok := <-sub; !ok {
			break
		}
	}
}
