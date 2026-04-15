package msauth

// Microsoft Graph API scopes
const (
	ScopeMail            = "Mail.ReadWrite"
	ScopeMailSend        = "Mail.Send"
	ScopeCalendar        = "Calendars.ReadWrite"
	ScopeContacts        = "Contacts.ReadWrite"
	ScopeTasks           = "Tasks.ReadWrite"
	ScopeFiles           = "Files.ReadWrite"
	ScopePeople          = "People.Read"
	ScopeUser            = "User.Read"
	ScopeUserReadAll     = "User.ReadBasic.All"
	ScopeMailboxSettings = "MailboxSettings.ReadWrite"
	ScopeOfflineAccess   = "offline_access"
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
		ScopeFiles,
		ScopePeople,
	}
}

// EnterpriseScopes returns all scopes including enterprise-only ones.
// Personal Microsoft accounts cannot consent to User.ReadBasic.All
// or MailboxSettings.ReadWrite — requesting them causes device code
// flow to fail with a misleading "code expired" error.
func EnterpriseScopes() []string {
	return append(DefaultScopes(), ScopeUserReadAll, ScopeMailboxSettings)
}

func ReadOnlyScopes() []string {
	return []string{
		ScopeOfflineAccess,
		ScopeUser,
		"Mail.Read",
		"Calendars.Read",
		"Contacts.Read",
		"Tasks.Read",
		"Files.Read",
		"People.Read",
	}
}

// EnterpriseReadOnlyScopes returns read-only scopes including enterprise-only ones.
func EnterpriseReadOnlyScopes() []string {
	return append(ReadOnlyScopes(), "User.ReadBasic.All", "MailboxSettings.Read")
}
