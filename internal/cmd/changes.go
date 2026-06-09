package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/outfmt"
)

// resourceChanges is one resource's slice of a unified changes digest: the
// changed items plus that resource's own independent continuation token.
type resourceChanges[T any] struct {
	Items      []T    `json:"items"`
	DeltaToken string `json:"deltaToken,omitempty"`
	Complete   bool   `json:"complete"`
}

// changesDigest is the cross-resource delta snapshot: mail, calendar, and
// contacts changes in one call, each with its own independent token.
type changesDigest struct {
	Mail     resourceChanges[graphapi.MailDelta]     `json:"mail"`
	Calendar resourceChanges[graphapi.CalendarDelta] `json:"calendar"`
	Contacts resourceChanges[graphapi.ContactDelta]  `json:"contacts"`
}

// ChangesCmd returns a unified digest of mail, calendar, and contacts changes in
// one call, each with its own delta token. Omit a token for that resource's
// initial sync; pass it back next time to get only what changed.
type ChangesCmd struct {
	MailToken     string `help:"Delta token for mail (omit for a fresh sync)" name:"mail-token"`
	CalendarToken string `help:"Delta token for calendar (omit for a fresh sync)" name:"calendar-token"`
	ContactsToken string `help:"Delta token for contacts (omit for a fresh sync)" name:"contacts-token"`
	Days          int    `help:"Calendar window in days ahead for the initial sync" default:"30"`
	Top           int32  `help:"Max items per resource page" short:"n"`
}

func (c *ChangesCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}

	var digest changesDigest

	mail, mp, err := client.DeltaMessages(ctx.Ctx, target, "inbox", c.MailToken, c.Top)
	if err != nil {
		return fmt.Errorf("mail delta: %w", err)
	}
	digest.Mail = resourceChanges[graphapi.MailDelta]{Items: mail, DeltaToken: mp.Token, Complete: mp.Complete}

	days := c.Days
	if days <= 0 {
		days = 30
	}
	start := time.Now()
	events, cp, err := client.DeltaCalendarView(ctx.Ctx, target, c.CalendarToken, start, start.AddDate(0, 0, days), c.Top)
	if err != nil {
		return fmt.Errorf("calendar delta: %w", err)
	}
	digest.Calendar = resourceChanges[graphapi.CalendarDelta]{Items: events, DeltaToken: cp.Token, Complete: cp.Complete}

	contacts, kp, err := client.DeltaContacts(ctx.Ctx, target, c.ContactsToken, c.Top)
	if err != nil {
		return fmt.Errorf("contacts delta: %w", err)
	}
	digest.Contacts = resourceChanges[graphapi.ContactDelta]{Items: contacts, DeltaToken: kp.Token, Complete: kp.Complete}

	total := len(mail) + len(events) + len(contacts)
	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(digest, total, "")
	}

	// Human/table view: a compact (id, change, summary) section per resource,
	// each followed by its token line.
	loc, _ := ctx.Timezone()
	mailRows := make([][]string, 0, len(mail))
	for i := range mail {
		m := &mail[i]
		mailRows = append(mailRows, []string{outfmt.Truncate(outfmt.Sanitize(m.ID), 15), changeLabel(m.Removed), outfmt.Truncate(outfmt.Sanitize(m.Subject), 50)})
	}
	if err := printChangesSection(printer, "MAIL", mailRows, mp); err != nil {
		return err
	}
	evRows := make([][]string, 0, len(events))
	for i := range events {
		e := &events[i]
		summary := outfmt.Sanitize(e.Subject) + " @ " + outfmt.Sanitize(outfmt.ConvertTime(e.Start, loc))
		evRows = append(evRows, []string{outfmt.Truncate(outfmt.Sanitize(e.ID), 15), changeLabel(e.Removed), outfmt.Truncate(summary, 50)})
	}
	if err := printChangesSection(printer, "CALENDAR", evRows, cp); err != nil {
		return err
	}
	ctRows := make([][]string, 0, len(contacts))
	for i := range contacts {
		ct := &contacts[i]
		summary := outfmt.Sanitize(ct.DisplayName) + " " + outfmt.Sanitize(strings.Join(ct.Emails, ", "))
		ctRows = append(ctRows, []string{outfmt.Truncate(outfmt.Sanitize(ct.ID), 15), changeLabel(ct.Removed), outfmt.Truncate(summary, 50)})
	}
	return printChangesSection(printer, "CONTACTS", ctRows, kp)
}

// printChangesSection renders one resource's table plus its token line.
func printChangesSection(printer *outfmt.Printer, title string, rows [][]string, page graphapi.DeltaPage) error {
	fmt.Printf("== %s (%d) ==\n", title, len(rows))
	if err := printer.Print([]string{"ID", "CHANGE", "SUMMARY"}, rows, nil, len(rows), ""); err != nil {
		return err
	}
	fmt.Printf("token: %s  complete: %v\n\n", page.Token, page.Complete)
	return nil
}
