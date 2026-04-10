package msauth

// Microsoft Graph API scopes
const (
	ScopeMail          = "Mail.ReadWrite"
	ScopeMailSend      = "Mail.Send"
	ScopeCalendar      = "Calendars.ReadWrite"
	ScopeContacts      = "Contacts.ReadWrite"
	ScopeTasks         = "Tasks.ReadWrite"
	ScopePeople           = "People.Read"
	ScopeUser             = "User.Read"
	ScopeUserReadAll      = "User.ReadBasic.All"
	ScopeMailboxSettings  = "MailboxSettings.ReadWrite"
	ScopeOfflineAccess    = "offline_access"
)

func DefaultScopes() []string {
	return []string{
		ScopeOfflineAccess,
		ScopeUser,
		ScopeMail,
		ScopeMailSend,
		ScopeCalendar,
		ScopeContacts,
		ScopeTasks,
		ScopePeople,
		ScopeUserReadAll,
		ScopeMailboxSettings,
	}
}

func ReadOnlyScopes() []string {
	return []string{
		ScopeOfflineAccess,
		ScopeUser,
		"Mail.Read",
		"Calendars.Read",
		"Contacts.Read",
		"Tasks.Read",
		"People.Read",
		"User.ReadBasic.All",
		"MailboxSettings.Read",
	}
}
