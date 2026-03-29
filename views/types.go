package views

// UserRow is a single row for the users table template.
type UserRow struct {
	Username string
	IsAdmin  bool
	Disabled bool
}
