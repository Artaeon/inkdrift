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

func TestDeleteListWithCampaigns(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, err := db.AddSubscriber("a@example.com", "", list.ID)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := db.CreateCampaign("Campaign", "Sub", "Body", list.ID)
	if err := db.LogSend(c.ID, sub.ID, "sent", ""); err != nil {
		t.Fatalf("LogSend should succeed with valid subscriber: %v", err)
	}

	// Should succeed even with campaigns and send logs
	if err := db.DeleteList(list.ID); err != nil {
		t.Fatalf("delete list with campaigns should succeed: %v", err)
	}

	_, err = db.GetCampaign(c.ID)
	if err == nil {
		t.Error("expected campaign to be deleted with list")
	}
}

func TestDeleteCampaignCascade(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, _ := db.AddSubscriber("test@example.com", "", list.ID)
	c, _ := db.CreateCampaign("Test", "Sub", "Body", list.ID)

	// Log a send with valid subscriber
	if err := db.LogSend(c.ID, sub.ID, "sent", ""); err != nil {
		t.Fatalf("LogSend should succeed: %v", err)
	}

	if err := db.DeleteCampaign(c.ID); err != nil {
		t.Fatal(err)
	}

	_, err := db.GetCampaign(c.ID)
	if err == nil {
		t.Error("expected error getting deleted campaign")
	}
}

func TestDeleteSubscriberWithSendLog(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, _ := db.AddSubscriber("test@example.com", "", list.ID)
	c, _ := db.CreateCampaign("Test", "Sub", "Body", list.ID)
	db.LogSend(c.ID, sub.ID, "sent", "")

	// Should succeed despite send_log referencing this subscriber
	if err := db.DeleteSubscriber(sub.ID); err != nil {
		t.Fatalf("delete subscriber with send_log should succeed: %v", err)
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

func TestStatusTransitionPartialToSending(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	c, _ := db.CreateCampaign("Test", "Sub", "Body", list.ID)

	db.UpdateCampaignStatus(c.ID, "sending")
	db.UpdateCampaignStatus(c.ID, "partial")

	if err := db.UpdateCampaignStatus(c.ID, "sending"); err != nil {
		t.Errorf("expected partial->sending, got: %v", err)
	}
}

func TestStatusTransitionFailedToSending(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	c, _ := db.CreateCampaign("Test", "Sub", "Body", list.ID)

	db.UpdateCampaignStatus(c.ID, "sending")
	db.UpdateCampaignStatus(c.ID, "failed")

	if err := db.UpdateCampaignStatus(c.ID, "sending"); err != nil {
		t.Errorf("expected failed->sending, got: %v", err)
	}
}

func TestStatusTransitionSendingToSending(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	c, _ := db.CreateCampaign("Test", "Sub", "Body", list.ID)

	db.UpdateCampaignStatus(c.ID, "sending")

	// sending->sending allowed (stuck campaign retry)
	if err := db.UpdateCampaignStatus(c.ID, "sending"); err != nil {
		t.Errorf("expected sending->sending, got: %v", err)
	}
}

func TestUpdateCampaignStatusNonexistent(t *testing.T) {
	db := testDB(t)

	if err := db.UpdateCampaignStatus("nonexistent", "sending"); err == nil {
		t.Error("expected error for nonexistent campaign")
	}
}

func TestUpdateCampaignStats(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	c, _ := db.CreateCampaign("Test", "Sub", "Body", list.ID)

	if err := db.UpdateCampaignStats(c.ID, 10, 2); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetCampaign(c.ID)
	if got.SentCount != 10 {
		t.Errorf("expected sent_count 10, got %d", got.SentCount)
	}
	if got.FailedCount != 2 {
		t.Errorf("expected failed_count 2, got %d", got.FailedCount)
	}
	if got.SentAt == nil {
		t.Error("expected sent_at to be set")
	}
}

func TestUpdateCampaignBody(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	c, _ := db.CreateCampaign("Test", "Old Subject", "<p>Old</p>", list.ID)

	updated, err := db.UpdateCampaignBody(c.ID, "New Subject", "<p>New</p>")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Subject != "New Subject" {
		t.Errorf("expected subject 'New Subject', got %q", updated.Subject)
	}
	if updated.Body != "<p>New</p>" {
		t.Errorf("expected body '<p>New</p>', got %q", updated.Body)
	}
}

func TestUpdateCampaignBodyNonDraft(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	c, _ := db.CreateCampaign("Test", "Sub", "Body", list.ID)
	db.ClaimCampaignForSending(c.ID) // Move to sending

	updated, err := db.UpdateCampaignBody(c.ID, "New", "New Body")
	if err != nil {
		t.Fatal(err)
	}
	// Body should not change since campaign is not draft
	if updated.Body == "New Body" {
		t.Error("non-draft campaign body should not be updated")
	}
}

func TestSetCampaignTemplate(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	c, _ := db.CreateCampaign("Test", "Sub", "Body", list.ID)
	tmpl, _ := db.CreateTemplate("Test Template", "<html>{{.Content}}</html>")

	if err := db.SetCampaignTemplate(c.ID, tmpl.ID); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetCampaign(c.ID)
	if got.TemplateID != tmpl.ID {
		t.Errorf("expected template_id %q, got %q", tmpl.ID, got.TemplateID)
	}
}

func TestListCampaigns(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	db.CreateCampaign("A", "Sub A", "Body A", list.ID)
	db.CreateCampaign("B", "Sub B", "Body B", list.ID)

	campaigns, err := db.ListCampaigns()
	if err != nil {
		t.Fatal(err)
	}
	if len(campaigns) != 2 {
		t.Errorf("expected 2 campaigns, got %d", len(campaigns))
	}
}

func TestLogSendAndGetSentSubscriberIDs(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub1, _ := db.AddSubscriber("a@example.com", "", list.ID)
	sub2, _ := db.AddSubscriber("b@example.com", "", list.ID)
	c, _ := db.CreateCampaign("Test", "Sub", "Body", list.ID)

	db.LogSend(c.ID, sub1.ID, "sent", "")
	db.LogSend(c.ID, sub2.ID, "failed", "SMTP error")

	sent, err := db.GetSentSubscriberIDs(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !sent[sub1.ID] {
		t.Error("sub1 should be in sent set")
	}
	if sent[sub2.ID] {
		t.Error("sub2 should not be in sent set (failed)")
	}
}

func TestGetSentSubscriberIDsEmpty(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	c, _ := db.CreateCampaign("Test", "Sub", "Body", list.ID)

	sent, err := db.GetSentSubscriberIDs(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sent) != 0 {
		t.Errorf("expected empty map, got %d entries", len(sent))
	}
}

// Template tests

func TestCreateAndGetTemplate(t *testing.T) {
	db := testDB(t)

	tmpl, err := db.CreateTemplate("My Template", "<html>{{.Content}}</html>")
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.Name != "My Template" {
		t.Errorf("expected name 'My Template', got %q", tmpl.Name)
	}

	got, err := db.GetTemplate(tmpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "<html>{{.Content}}</html>" {
		t.Errorf("expected body '<html>{{.Content}}</html>', got %q", got.Body)
	}
}

func TestGetTemplateByName(t *testing.T) {
	db := testDB(t)

	db.CreateTemplate("Named", "<p>Body</p>")

	got, err := db.GetTemplateByName("Named")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Named" {
		t.Errorf("expected name 'Named', got %q", got.Name)
	}
}

func TestGetTemplateByNameNotFound(t *testing.T) {
	db := testDB(t)

	_, err := db.GetTemplateByName("Nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent template")
	}
}

func TestListTemplates(t *testing.T) {
	db := testDB(t)

	db.CreateTemplate("Alpha", "<p>A</p>")
	db.CreateTemplate("Beta", "<p>B</p>")

	templates, err := db.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 2 {
		t.Errorf("expected 2 templates, got %d", len(templates))
	}
	// Should be ordered by name
	if templates[0].Name != "Alpha" {
		t.Errorf("expected first template 'Alpha', got %q", templates[0].Name)
	}
}

func TestUpdateTemplate(t *testing.T) {
	db := testDB(t)

	tmpl, _ := db.CreateTemplate("Old", "<p>Old</p>")
	if err := db.UpdateTemplate(tmpl.ID, "New", "<p>New</p>"); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetTemplate(tmpl.ID)
	if got.Name != "New" {
		t.Errorf("expected name 'New', got %q", got.Name)
	}
	if got.Body != "<p>New</p>" {
		t.Errorf("expected body '<p>New</p>', got %q", got.Body)
	}
}

func TestDeleteTemplate(t *testing.T) {
	db := testDB(t)

	tmpl, _ := db.CreateTemplate("Delete Me", "<p>X</p>")
	if err := db.DeleteTemplate(tmpl.ID); err != nil {
		t.Fatal(err)
	}

	_, err := db.GetTemplate(tmpl.ID)
	if err == nil {
		t.Error("expected error getting deleted template")
	}
}

func TestTemplateSizeLimit(t *testing.T) {
	db := testDB(t)

	bigBody := make([]byte, maxTemplateBodySize+1)
	for i := range bigBody {
		bigBody[i] = 'a'
	}

	_, err := db.CreateTemplate("Big", string(bigBody))
	if err == nil {
		t.Error("expected error for oversized template body")
	}
}

// Subscriber tests

func TestGetSubscriber(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")

	sub, _ := db.AddSubscriber("test@example.com", "Test", list.ID)

	got, err := db.GetSubscriber(sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got %q", got.Email)
	}
	if got.Name != "Test" {
		t.Errorf("expected name 'Test', got %q", got.Name)
	}
}

func TestGetSubscriberNotFound(t *testing.T) {
	db := testDB(t)

	_, err := db.GetSubscriber("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent subscriber")
	}
}

func TestGetSubscriberByEmail(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	db.AddSubscriber("find@example.com", "", list.ID)

	got, err := db.GetSubscriberByEmail("find@example.com", list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "find@example.com" {
		t.Errorf("expected email 'find@example.com', got %q", got.Email)
	}
}

func TestGetSubscriberByEmailNotFound(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")

	_, err := db.GetSubscriberByEmail("missing@example.com", list.ID)
	if err == nil {
		t.Error("expected error for missing subscriber")
	}
}

func TestGetSubscriberByToken(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, _ := db.AddSubscriber("test@example.com", "", list.ID)

	got, err := db.GetSubscriberByToken(sub.ConfirmToken)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != sub.ID {
		t.Errorf("expected ID %q, got %q", sub.ID, got.ID)
	}
}

func TestGetSubscriberByTokenNotFound(t *testing.T) {
	db := testDB(t)

	_, err := db.GetSubscriberByToken("nonexistent-token")
	if err == nil {
		t.Error("expected error for nonexistent token")
	}
}

func TestListSubscribers(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	db.AddSubscriber("a@example.com", "", list.ID)
	db.AddSubscriber("b@example.com", "", list.ID)

	subs, err := db.ListSubscribers(list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Errorf("expected 2 subscribers, got %d", len(subs))
	}
}

func TestListSubscribersPaginated(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	for i := 0; i < 5; i++ {
		db.AddSubscriber(string(rune('a'+i))+"@example.com", "", list.ID)
	}

	// Limit 2, offset 0
	subs, err := db.ListSubscribersPaginated(list.ID, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Errorf("expected 2 subscribers, got %d", len(subs))
	}

	// Limit 2, offset 3
	subs, _ = db.ListSubscribersPaginated(list.ID, 2, 3)
	if len(subs) != 2 {
		t.Errorf("expected 2 subscribers with offset, got %d", len(subs))
	}

	// Invalid limit defaults to 100
	subs, _ = db.ListSubscribersPaginated(list.ID, 0, 0)
	if len(subs) != 5 {
		t.Errorf("expected 5 subscribers with default limit, got %d", len(subs))
	}

	// Negative offset defaults to 0
	subs, _ = db.ListSubscribersPaginated(list.ID, 100, -1)
	if len(subs) != 5 {
		t.Errorf("expected 5 subscribers with default offset, got %d", len(subs))
	}
}

func TestGetActiveSubscribers(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	db.AddSubscriber("active@example.com", "", list.ID)
	db.AddSubscriberWithStatus("pending@example.com", "", list.ID, "pending")

	subs, err := db.GetActiveSubscribers(list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 {
		t.Errorf("expected 1 active subscriber, got %d", len(subs))
	}
	if subs[0].Email != "active@example.com" {
		t.Errorf("expected 'active@example.com', got %q", subs[0].Email)
	}
}

func TestConfirmSubscriber(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, _ := db.AddSubscriberWithStatus("test@example.com", "", list.ID, "pending")

	if err := db.ConfirmSubscriber(sub.ConfirmToken); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSubscriber(sub.ID)
	if got.Status != "active" {
		t.Errorf("expected status 'active', got %q", got.Status)
	}
	if !got.Confirmed {
		t.Error("expected confirmed=true")
	}
}

func TestConfirmSubscriberInvalidToken(t *testing.T) {
	db := testDB(t)

	if err := db.ConfirmSubscriber("invalid-token"); err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestConfirmSubscriberAlreadyActive(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, _ := db.AddSubscriber("test@example.com", "", list.ID) // active by default

	// Should still succeed (confirms already active subscriber)
	if err := db.ConfirmSubscriber(sub.ConfirmToken); err != nil {
		t.Errorf("confirming active subscriber should succeed: %v", err)
	}
}

func TestUnsubscribeByToken(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, _ := db.AddSubscriber("test@example.com", "", list.ID)

	if err := db.UnsubscribeByToken(sub.ConfirmToken); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSubscriber(sub.ID)
	if got.Status != "unsubscribed" {
		t.Errorf("expected status 'unsubscribed', got %q", got.Status)
	}
	if got.UnsubscribedAt == nil {
		t.Error("expected unsubscribed_at to be set")
	}
}

func TestUnsubscribeByTokenInvalid(t *testing.T) {
	db := testDB(t)

	if err := db.UnsubscribeByToken("invalid-token"); err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestUnsubscribeByTokenAlreadyUnsubscribed(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, _ := db.AddSubscriber("test@example.com", "", list.ID)

	// First unsubscribe
	db.UnsubscribeByToken(sub.ConfirmToken)

	// Second unsubscribe should fail
	if err := db.UnsubscribeByToken(sub.ConfirmToken); err == nil {
		t.Error("expected error for already unsubscribed")
	}
}

func TestMarkBounced(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, _ := db.AddSubscriber("test@example.com", "", list.ID)

	if err := db.MarkBounced(sub.ID); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSubscriber(sub.ID)
	if got.Status != "bounced" {
		t.Errorf("expected status 'bounced', got %q", got.Status)
	}
}

func TestMarkBouncedNonActive(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, _ := db.AddSubscriberWithStatus("test@example.com", "", list.ID, "pending")

	// MarkBounced only works on active subscribers
	db.MarkBounced(sub.ID)

	got, _ := db.GetSubscriber(sub.ID)
	if got.Status != "pending" {
		t.Errorf("pending subscriber should stay pending, got %q", got.Status)
	}
}

func TestSearchSubscribers(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	db.AddSubscriber("alice@example.com", "Alice", list.ID)
	db.AddSubscriber("bob@example.com", "Bob", list.ID)
	db.AddSubscriber("charlie@example.com", "Charlie", list.ID)

	// Search by email prefix
	subs, err := db.SearchSubscribers(list.ID, "alice", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 {
		t.Errorf("expected 1 result for 'alice', got %d", len(subs))
	}

	// Search by name prefix
	subs, _ = db.SearchSubscribers(list.ID, "Bob", 50)
	if len(subs) != 1 {
		t.Errorf("expected 1 result for 'Bob', got %d", len(subs))
	}

	// Limit
	subs, _ = db.SearchSubscribers(list.ID, "", 2)
	// Empty prefix matches nothing with LIKE
	// Actually "%" matches everything
	// Wait, pattern is query + "%" so empty query = "%" which matches all
	// But the limit is bounded
}

func TestSearchSubscribersDefaultLimit(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	db.AddSubscriber("test@example.com", "", list.ID)

	// Invalid limits should default to 50
	subs, _ := db.SearchSubscribers(list.ID, "test", 0)
	if len(subs) != 1 {
		t.Errorf("expected 1 result with default limit, got %d", len(subs))
	}

	subs, _ = db.SearchSubscribers(list.ID, "test", 200)
	if len(subs) != 1 {
		t.Errorf("expected 1 result with capped limit, got %d", len(subs))
	}
}

func TestImportSubscribers(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")

	entries := []struct{ Email, Name string }{
		{"import1@example.com", "Import 1"},
		{"import2@example.com", "Import 2"},
		{"import3@example.com", "Import 3"},
	}

	count, err := db.ImportSubscribers(list.ID, entries)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3 imported, got %d", count)
	}

	// Verify they exist
	subs, _ := db.ListSubscribers(list.ID)
	if len(subs) != 3 {
		t.Errorf("expected 3 subscribers, got %d", len(subs))
	}
}

func TestImportSubscribersDuplicates(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")

	db.AddSubscriber("existing@example.com", "", list.ID)

	entries := []struct{ Email, Name string }{
		{"existing@example.com", ""},   // duplicate - should be skipped
		{"new@example.com", "New User"}, // new
	}

	count, err := db.ImportSubscribers(list.ID, entries)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 imported (1 duplicate skipped), got %d", count)
	}
}

func TestResubscribePending(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, _ := db.AddSubscriber("test@example.com", "", list.ID)

	// Unsubscribe first
	db.UnsubscribeByToken(sub.ConfirmToken)

	// Resubscribe as pending
	if err := db.ResubscribePending(sub.ID); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSubscriber(sub.ID)
	if got.Status != "pending" {
		t.Errorf("expected status 'pending', got %q", got.Status)
	}
	if got.Confirmed {
		t.Error("expected confirmed=false after resubscribe pending")
	}
	if got.UnsubscribedAt != nil {
		t.Error("expected unsubscribed_at to be cleared")
	}
	// Token should have been regenerated
	if got.ConfirmToken == sub.ConfirmToken {
		t.Error("expected new confirm token after resubscribe")
	}
}

func TestResubscribePendingActiveUser(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, _ := db.AddSubscriber("test@example.com", "", list.ID)

	// Active subscriber can't resubscribe
	if err := db.ResubscribePending(sub.ID); err == nil {
		t.Error("expected error resubscribing active subscriber")
	}
}

func TestResubscribeActive(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, _ := db.AddSubscriber("test@example.com", "", list.ID)

	db.UnsubscribeByToken(sub.ConfirmToken)

	if err := db.ResubscribeActive(sub.ID); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSubscriber(sub.ID)
	if got.Status != "active" {
		t.Errorf("expected status 'active', got %q", got.Status)
	}
	if !got.Confirmed {
		t.Error("expected confirmed=true after resubscribe active")
	}
}

func TestResubscribeActiveFromBounced(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, _ := db.AddSubscriber("test@example.com", "", list.ID)

	db.MarkBounced(sub.ID)

	if err := db.ResubscribeActive(sub.ID); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSubscriber(sub.ID)
	if got.Status != "active" {
		t.Errorf("expected status 'active', got %q", got.Status)
	}
}

func TestResubscribeActiveAlreadyActive(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")
	sub, _ := db.AddSubscriber("test@example.com", "", list.ID)

	if err := db.ResubscribeActive(sub.ID); err == nil {
		t.Error("expected error resubscribing already active subscriber")
	}
}

func TestAddSubscriberWithStatus(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")

	sub, err := db.AddSubscriberWithStatus("test@example.com", "Test", list.ID, "pending")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Status != "pending" {
		t.Errorf("expected status 'pending', got %q", sub.Status)
	}
	if sub.Confirmed {
		t.Error("expected confirmed=false for pending subscriber")
	}
	if sub.ConfirmToken == "" {
		t.Error("expected non-empty confirm token")
	}
}

func TestAddSubscriberActiveIsConfirmed(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")

	sub, _ := db.AddSubscriber("test@example.com", "", list.ID)
	if !sub.Confirmed {
		t.Error("expected confirmed=true for active subscriber")
	}
}

// List tests

func TestGetListNotFound(t *testing.T) {
	db := testDB(t)

	_, err := db.GetList("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent list")
	}
}

func TestGetListByName(t *testing.T) {
	db := testDB(t)
	db.CreateList("Find Me", "desc")

	got, err := db.GetListByName("Find Me")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Find Me" {
		t.Errorf("expected 'Find Me', got %q", got.Name)
	}
}

func TestGetListByNameNotFound(t *testing.T) {
	db := testDB(t)

	_, err := db.GetListByName("Nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent list name")
	}
}

func TestListLists(t *testing.T) {
	db := testDB(t)
	db.CreateList("Alpha", "")
	db.CreateList("Beta", "")

	lists, err := db.ListLists()
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 2 {
		t.Errorf("expected 2 lists, got %d", len(lists))
	}
	// Should be ordered by name
	if lists[0].Name != "Alpha" {
		t.Errorf("expected first list 'Alpha', got %q", lists[0].Name)
	}
}

func TestDuplicateListName(t *testing.T) {
	db := testDB(t)
	db.CreateList("Unique", "")

	_, err := db.CreateList("Unique", "")
	if err == nil {
		t.Error("expected error for duplicate list name")
	}
}

func TestGetCampaignNotFound(t *testing.T) {
	db := testDB(t)

	_, err := db.GetCampaign("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent campaign")
	}
}

func TestSubjectSizeLimit(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")

	longSubject := make([]byte, maxSubjectSize+1)
	for i := range longSubject {
		longSubject[i] = 'a'
	}

	_, err := db.CreateCampaign("Test", string(longSubject), "Body", list.ID)
	if err == nil {
		t.Error("expected error for oversized subject")
	}
}

// Database open/close tests

func TestOpenInvalidPath(t *testing.T) {
	_, err := Open("/nonexistent/dir/test.db")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestSubscriberCountsAllStatuses(t *testing.T) {
	db := testDB(t)
	list, _ := db.CreateList("Test", "")

	db.AddSubscriber("active@example.com", "", list.ID)
	db.AddSubscriberWithStatus("pending@example.com", "", list.ID, "pending")

	sub3, _ := db.AddSubscriber("unsub@example.com", "", list.ID)
	db.UnsubscribeByEmail("unsub@example.com", list.ID)

	sub4, _ := db.AddSubscriber("bounced@example.com", "", list.ID)
	db.MarkBounced(sub4.ID)
	_ = sub3

	counts, err := db.GetSubscriberCounts(list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Active != 1 {
		t.Errorf("expected 1 active, got %d", counts.Active)
	}
	if counts.Pending != 1 {
		t.Errorf("expected 1 pending, got %d", counts.Pending)
	}
	if counts.Unsubscribed != 1 {
		t.Errorf("expected 1 unsubscribed, got %d", counts.Unsubscribed)
	}
	if counts.Bounced != 1 {
		t.Errorf("expected 1 bounced, got %d", counts.Bounced)
	}
	if counts.Total != 4 {
		t.Errorf("expected 4 total, got %d", counts.Total)
	}
}

func TestClaimCampaignNonexistent(t *testing.T) {
	db := testDB(t)

	if err := db.ClaimCampaignForSending("nonexistent"); err == nil {
		t.Error("expected error for nonexistent campaign")
	}
}
