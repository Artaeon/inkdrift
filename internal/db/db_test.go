package db

import (
	"os"
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	f, err := os.CreateTemp("", "inkdrift-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateAndGetList(t *testing.T) {
	db := testDB(t)

	list, err := db.CreateList("Test List", "A test list")
	if err != nil {
		t.Fatal(err)
	}
	if list.Name != "Test List" {
		t.Errorf("expected name 'Test List', got %q", list.Name)
	}

	got, err := db.GetList(list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Test List" {
		t.Errorf("expected name 'Test List', got %q", got.Name)
	}
}

func TestCreateAndGetCampaign(t *testing.T) {
	db := testDB(t)

	list, err := db.CreateList("Test", "")
	if err != nil {
		t.Fatal(err)
	}

	c, err := db.CreateCampaign("My Campaign", "Subject", "<p>Body</p>", list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != "draft" {
		t.Errorf("expected status 'draft', got %q", c.Status)
	}

	got, err := db.GetCampaign(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "Subject" {
		t.Errorf("expected subject 'Subject', got %q", got.Subject)
	}
}

func TestCampaignBodySizeLimit(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")

	bigBody := make([]byte, maxCampaignBodySize+1)
	for i := range bigBody {
		bigBody[i] = 'a'
	}

	_, err := db.CreateCampaign("Big", "Sub", string(bigBody), list.ID)
	if err == nil {
		t.Error("expected error for oversized body")
	}
}

func TestClaimCampaignForSending(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	c, _ := db.CreateCampaign("Test", "Sub", "Body", list.ID)

	// First claim should succeed
	if err := db.ClaimCampaignForSending(c.ID); err != nil {
		t.Fatal(err)
	}

	// Second claim should fail (no longer draft)
	if err := db.ClaimCampaignForSending(c.ID); err == nil {
		t.Error("expected error on double claim")
	}
}

func TestSubscriberLifecycle(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")

	// Add subscriber
	sub, err := db.AddSubscriber("test@example.com", "Test User", list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sub.Status != "active" {
		t.Errorf("expected status 'active', got %q", sub.Status)
	}

	// Count
	count, err := db.ListSubscriberCount(list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 subscriber, got %d", count)
	}

	// Unsubscribe
	if err := db.UnsubscribeByEmail("test@example.com", list.ID); err != nil {
		t.Fatal(err)
	}

	count, _ = db.ListSubscriberCount(list.ID)
	if count != 0 {
		t.Errorf("expected 0 active subscribers after unsubscribe, got %d", count)
	}

	// Resubscribe
	if err := db.ResubscribeActive(sub.ID); err != nil {
		t.Fatal(err)
	}
	count, _ = db.ListSubscriberCount(list.ID)
	if count != 1 {
		t.Errorf("expected 1 active subscriber after resubscribe, got %d", count)
	}
}

func TestSubscriberCounts(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")

	db.AddSubscriber("a@example.com", "", list.ID)
	db.AddSubscriber("b@example.com", "", list.ID)
	db.AddSubscriberWithStatus("c@example.com", "", list.ID, "pending")

	counts, err := db.GetSubscriberCounts(list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Active != 2 {
		t.Errorf("expected 2 active, got %d", counts.Active)
	}
	if counts.Pending != 1 {
		t.Errorf("expected 1 pending, got %d", counts.Pending)
	}
	if counts.Total != 3 {
		t.Errorf("expected 3 total, got %d", counts.Total)
	}
}

func TestDuplicateSubscriber(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")

	_, err := db.AddSubscriber("test@example.com", "", list.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Duplicate should fail
	_, err = db.AddSubscriber("test@example.com", "", list.ID)
	if err == nil {
		t.Error("expected error for duplicate subscriber")
	}
}

func TestDeleteListCascade(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	db.AddSubscriber("a@example.com", "", list.ID)

	if err := db.DeleteList(list.ID); err != nil {
		t.Fatal(err)
	}

	_, err := db.GetList(list.ID)
	if err == nil {
		t.Error("expected error getting deleted list")
	}
}

func TestDeleteCampaignCascade(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	c, _ := db.CreateCampaign("Test", "Sub", "Body", list.ID)

	// Log a send
	db.LogSend(c.ID, "fake-sub-id", "sent", "")

	if err := db.DeleteCampaign(c.ID); err != nil {
		t.Fatal(err)
	}

	_, err := db.GetCampaign(c.ID)
	if err == nil {
		t.Error("expected error getting deleted campaign")
	}
}

func TestStatusValidation(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	c, _ := db.CreateCampaign("Test", "Sub", "Body", list.ID)

	// Invalid status name
	if err := db.UpdateCampaignStatus(c.ID, "invalid"); err == nil {
		t.Error("expected error for invalid status")
	}

	// Valid transition: draft -> sending
	if err := db.UpdateCampaignStatus(c.ID, "sending"); err != nil {
		t.Errorf("expected valid transition draft->sending, got: %v", err)
	}

	// Valid transition: sending -> sent
	if err := db.UpdateCampaignStatus(c.ID, "sent"); err != nil {
		t.Errorf("expected valid transition sending->sent, got: %v", err)
	}

	// Invalid transition: sent is terminal
	if err := db.UpdateCampaignStatus(c.ID, "draft"); err == nil {
		t.Error("expected error transitioning from sent to draft")
	}
}
