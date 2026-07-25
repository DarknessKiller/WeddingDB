func (e unauthorizedError) Error() string { return e.msg }

type forbiddenError struct{ msg string }
