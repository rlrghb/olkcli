package cmd

type MailCmd struct {
	List        MailListCmd        `cmd:"" help:"List messages in inbox"`
	Get         MailGetCmd         `cmd:"" help:"Get a message"`
	Send        MailSendCmd        `cmd:"" help:"Send a message"`
	Search      MailSearchCmd      `cmd:"" help:"Search messages"`
	Reply       MailReplyCmd       `cmd:"" help:"Reply to a message"`
	Forward     MailForwardCmd     `cmd:"" help:"Forward a message"`
	Move        MailMoveCmd        `cmd:"" help:"Move a message to a folder"`
	Delete      MailDeleteCmd      `cmd:"" help:"Delete a message"`
	Mark        MailMarkCmd        `cmd:"" help:"Mark message as read/unread"`
	Folders     MailFoldersCmd     `cmd:"" help:"List mail folders"`
	Attachments MailAttachmentsCmd `cmd:"" help:"List/download attachments"`
	Drafts      MailDraftsCmd      `cmd:"" help:"Draft message operations"`
	Flag        MailFlagCmd        `cmd:"" help:"Set follow-up flag on a message"`
	Importance  MailImportanceCmd  `cmd:"" help:"Set importance on a message"`
	Categorize  MailCategorizeCmd  `cmd:"" help:"Set categories on a message"`
	OOO         MailOOOCmd        `cmd:"" help:"Out-of-office / auto-reply settings"`
}
