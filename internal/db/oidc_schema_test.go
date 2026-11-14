package db

import (
	"path/filepath"
	"testing"
)

func TestFreshOIDCIdentitySchemaAndLookup(t *testing.T) {
	database, err := InitDB(filepath.Join(t.TempDir(), "identity.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})

	providerID := "idp-regression"
	provider := OIDCProvider{
		ID: providerID, Name: "OIDC Regression", ProviderType: "custom",
		IssuerURL: "https://identity.example.test", ClientID: "test-client",
	}
	if err := database.Create(&provider).Error; err != nil {
		t.Fatal(err)
	}
	user := User{
		ID: "usr-oidc-regression", Email: "regression@example.test",
		Name: "OIDC Regression", AuthType: "oidc", Status: "PENDING",
		IDPProviderID: &providerID, IDPSubject: "external-subject",
	}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	var got User
	if err := database.Where(
		"idp_provider_id = ? AND idp_subject = ? AND auth_type = ?",
		providerID, user.IDPSubject, "oidc",
	).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.ID != user.ID {
		t.Fatalf("unexpected linked user: %s", got.ID)
	}
}

func TestFreshOIDCIdentitySchemaPreventsDuplicateProviderSubject(t *testing.T) {
	database, err := InitDB(filepath.Join(t.TempDir(), "unique-identity.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})
	provider := OIDCProvider{
		ID: "idp-one", Name: "Identity Provider", ProviderType: "custom",
		IssuerURL: "https://identity.example.test", ClientID: "client",
	}
	if err := database.Create(&provider).Error; err != nil {
		t.Fatal(err)
	}
	providerID := provider.ID
	first := User{
		ID: "user-one", Email: "one@example.test", Name: "User One",
		AuthType: "oidc", Status: "PENDING", IDPProviderID: &providerID,
		IDPSubject: "shared-subject",
	}
	if err := database.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	second := User{
		ID: "user-two", Email: "two@example.test", Name: "User Two",
		AuthType: "oidc", Status: "PENDING", IDPProviderID: &providerID,
		IDPSubject: "shared-subject",
	}
	if err := database.Create(&second).Error; err == nil {
		t.Fatal("duplicate OIDC provider/subject was accepted")
	}
}
