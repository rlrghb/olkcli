package cmd

// SendCmd is a shortcut for `olkcli mail send`
type SendCmd struct {
	To      []string `help:"Recipient email addresses" required:"" short:"t"`
	Subject string   `help:"Email subject" required:"" short:"s"`
	Body    string   `help:"Email body" short:"b"`
	CC      []string `help:"CC recipients"`
	BCC     []string `help:"BCC recipients"`
	HTML    bool     `help:"Send body as HTML"`
}

func (c *SendCmd) Run(ctx *RunContext) error {
	inner := &MailSendCmd{
		To:      c.To,
		Subject: c.Subject,
		Body:    c.Body,
		CC:      c.CC,
		BCC:     c.BCC,
		HTML:    c.HTML,
	}
	return inner.Run(ctx)
}

// LsCmd is a shortcut for `olkcli mail list`
type LsCmd struct {
	Folder string `help:"Mail folder" short:"f"`
	Top    int32  `help:"Number of messages" default:"25" short:"n"`
	Unread bool   `help:"Unread only" short:"u"`
}

func (c *LsCmd) Run(ctx *RunContext) error {
	inner := &MailListCmd{
		Folder: c.Folder,
		Top:    c.Top,
		Unread: c.Unread,
	}
	return inner.Run(ctx)
}

// InboxCmd is a shortcut for `olkcli mail list`
type InboxCmd struct {
	Top    int32 `help:"Number of messages" default:"25" short:"n"`
	Unread bool  `help:"Unread only" short:"u"`
}

func (c *InboxCmd) Run(ctx *RunContext) error {
	inner := &MailListCmd{
		Top:    c.Top,
		Unread: c.Unread,
	}
	return inner.Run(ctx)
}

// SearchCmd is a shortcut for `olkcli mail search`
type SearchCmd struct {
	Query string `arg:"" help:"Search query"`
	Top   int32  `help:"Max results" default:"25" short:"n"`
}

func (c *SearchCmd) Run(ctx *RunContext) error {
	inner := &MailSearchCmd{
		Query: c.Query,
		Top:   c.Top,
	}
	return inner.Run(ctx)
}

// TodayCmd is a shortcut for `olkcli calendar events --days 1`
type TodayCmd struct {
	Top int32 `help:"Max events" default:"25" short:"n"`
}

func (c *TodayCmd) Run(ctx *RunContext) error {
	inner := &CalendarEventsCmd{
		Days: 1,
		Top:  c.Top,
	}
	return inner.Run(ctx)
}

// WeekCmd is a shortcut for `olkcli calendar events --days 7`
type WeekCmd struct {
	Top int32 `help:"Max events" default:"25" short:"n"`
}

func (c *WeekCmd) Run(ctx *RunContext) error {
	inner := &CalendarEventsCmd{
		Days: 7,
		Top:  c.Top,
	}
	return inner.Run(ctx)
}
