package graphapi

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The capability guard is the first statement in every mutating method, so it
// returns before the (nil) inner client is ever touched. These representative
// calls cover each return shape (error, (T,error), (string,error)) and the send
// vs write distinction.

func TestNoWriteGuardBlocksMutations(t *testing.T) {
	c := &Client{noWrite: true}
	ctx := context.Background()

	checks := []struct {
		name string
		call func() error
	}{
		{"DeleteMessage", func() error { return c.DeleteMessage(ctx, "id") }},
		{"MoveMessage", func() error { _, err := c.MoveMessage(ctx, "id", "f"); return err }},
		{"DeleteEvent", func() error { return c.DeleteEvent(ctx, "id") }},
		{"DeleteContact", func() error { return c.DeleteContact(ctx, "id") }},
		{"DeleteDriveItem", func() error { return c.DeleteDriveItem(ctx, "d", "i") }},
		{"DeleteTodoTask", func() error { return c.DeleteTodoTask(ctx, "l", "t") }},
		{"CreateMailFolder", func() error { _, err := c.CreateMailFolder(ctx, "n"); return err }},
		{"CreateUploadSession", func() error { _, err := c.CreateUploadSession(ctx, "d", "p", false); return err }},
		{"CreateEvent", func() error {
			_, err := c.CreateEvent(ctx, "", "s", time.Now(), time.Now(), "", []string{"a@b.com"}, false, false, "", "", nil)
			return err
		}},
		{"SendMessage", func() error { return c.SendMessage(ctx, "", &SendMessageOptions{Subject: "s", Body: "b"}) }},
		// Draft writes can land in someone else's mailbox now, so the guard
		// matters more than when they could only touch your own.
		{"CreateDraft", func() error {
			_, err := c.CreateDraft(ctx, "", "s", "b", []string{"a@b.com"}, nil, nil, false)
			return err
		}},
		{"CreateDraft into a shared mailbox", func() error {
			_, err := c.CreateDraft(ctx, "shared@example.com", "s", "b", []string{"a@b.com"}, nil, nil, false)
			return err
		}},
		{"DeleteDraft", func() error { return c.DeleteDraft(ctx, "", "id") }},
		{"DeleteDraft from a shared mailbox", func() error { return c.DeleteDraft(ctx, "shared@example.com", "id") }},
	}
	for _, tc := range checks {
		if err := tc.call(); !errors.Is(err, ErrNoWrite) {
			t.Errorf("%s under --no-write = %v, want ErrNoWrite", tc.name, err)
		}
	}
}

func TestNoSendGuardBlocksSends(t *testing.T) {
	c := &Client{noSend: true} // read-only off; only sending blocked
	ctx := context.Background()

	sends := []struct {
		name string
		call func() error
	}{
		{"SendMessage", func() error { return c.SendMessage(ctx, "", &SendMessageOptions{Subject: "s", Body: "b"}) }},
		{"ReplyMessage", func() error { return c.ReplyMessage(ctx, "", "id", "c", false) }},
		{"ForwardMessage", func() error { return c.ForwardMessage(ctx, "", "id", "c", []string{"a@b.com"}) }},
		{"SendDraft", func() error { return c.SendDraft(ctx, "", "id") }},
		{"SendDraft from a shared mailbox", func() error { return c.SendDraft(ctx, "shared@example.com", "id") }},
		{"RespondToEvent", func() error { return c.RespondToEvent(ctx, "id", "accept") }},
		{"CreateEvent w/ attendees", func() error {
			_, err := c.CreateEvent(ctx, "", "s", time.Now(), time.Now(), "", []string{"a@b.com"}, false, false, "", "", nil)
			return err
		}},
	}
	for _, tc := range sends {
		if err := tc.call(); !errors.Is(err, ErrNoSend) {
			t.Errorf("%s under --no-send = %v, want ErrNoSend", tc.name, err)
		}
	}
}
