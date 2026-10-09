package evidence

// ValidationError rejects a request before durable acceptance. Code is a fixed,
// public rejection category; source values and decoder errors stay private.
type ValidationError struct{ Code string }

func (e *ValidationError) Error() string { return e.Code }
